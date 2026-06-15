package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// runDlq operates on failed gebweb_jobs rows directly via $DATABASE_URL, like migrate.
func runDlq(args []string) int {
	if hasHelpFlag(args) || len(args) == 0 {
		printDlqHelp(os.Stdout)
		return 0
	}
	op := args[0]
	ids, all := dlqTargets(args[1:])
	driver, _, derr := splitDSN(os.Getenv("DATABASE_URL"))
	db, err := openMigrationDB()
	if err != nil || derr != nil {
		if derr != nil {
			err = derr
		}
		fmt.Fprintf(os.Stderr, "gebweb worker dlq: %v\n", err)
		return 1
	}
	defer db.Close()

	switch op {
	case "list":
		if err := dlqList(db, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb worker dlq list: %v\n", err)
			return 1
		}
	case "retry":
		n, err := dlqRetry(db, driver, ids, all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gebweb worker dlq retry: %v\n", err)
			return 1
		}
		fmt.Printf("re-queued %d job(s)\n", n)
	case "purge":
		n, err := dlqPurge(db, driver, ids, all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gebweb worker dlq purge: %v\n", err)
			return 1
		}
		fmt.Printf("purged %d job(s)\n", n)
	default:
		fmt.Fprintf(os.Stderr, "gebweb worker dlq: unknown operation %q\n", op)
		printDlqHelp(os.Stderr)
		return 2
	}
	return 0
}

func dlqTargets(args []string) (ids []string, all bool) {
	for _, a := range args {
		if a == "--all" {
			all = true
		} else {
			ids = append(ids, a)
		}
	}
	return ids, all
}

func dlqPlaceholder(driver string) string {
	if driver == "pgx" {
		return "$1"
	}
	return "?"
}

func dlqList(db *sql.DB, w io.Writer) error {
	rows, err := db.Query("SELECT id, name, attempts, created_at, last_error FROM gebweb_jobs WHERE status = 'failed' ORDER BY created_at")
	if err != nil {
		return err
	}
	defer rows.Close()
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	count := 0
	for rows.Next() {
		var id, name, createdAt string
		var attempts int
		var lastErr sql.NullString
		if err := rows.Scan(&id, &name, &attempts, &createdAt, &lastErr); err != nil {
			return err
		}
		if count == 0 {
			fmt.Fprintln(tw, "ID\tNAME\tATTEMPTS\tCREATED\tERROR")
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", id, name, attempts, createdAt, lastErr.String)
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	tw.Flush()
	if count == 0 {
		fmt.Fprintln(w, "no failed jobs")
	}
	return nil
}

// dlqRetry re-queues failed jobs; run_at to the epoch makes them immediately due.
func dlqRetry(db *sql.DB, driver string, ids []string, all bool) (int, error) {
	const set = "UPDATE gebweb_jobs SET status = 'pending', attempts = 0, run_at = '1970-01-01T00:00:00Z', locked_at = NULL, locked_by = NULL, last_error = NULL WHERE status = 'failed'"
	return dlqMutate(db, driver, set, ids, all)
}

func dlqPurge(db *sql.DB, driver string, ids []string, all bool) (int, error) {
	return dlqMutate(db, driver, "DELETE FROM gebweb_jobs WHERE status = 'failed'", ids, all)
}

func dlqMutate(db *sql.DB, driver, baseSQL string, ids []string, all bool) (int, error) {
	if !all && len(ids) == 0 {
		return 0, fmt.Errorf("specify one or more job ids, or --all")
	}
	if all {
		res, err := db.Exec(baseSQL)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	total := 0
	stmt := baseSQL + " AND id = " + dlqPlaceholder(driver)
	for _, id := range ids {
		res, err := db.Exec(stmt, id)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

func printDlqHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: gebweb worker dlq <list|retry|purge> [job-id ...] [--all]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Inspect and recover jobs that exhausted their retries (status 'failed').")
	fmt.Fprintln(w, "Connects to $DATABASE_URL, like `gebweb migrate`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  list                    show failed jobs")
	fmt.Fprintln(w, "  retry <id>... | --all   re-queue failed jobs (attempts reset)")
	fmt.Fprintln(w, "  purge <id>... | --all   delete failed jobs")
}
