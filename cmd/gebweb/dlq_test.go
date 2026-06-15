package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dlqTestDB seeds a gebweb_jobs table with failed j1/j2, pending p1, completed c1.
func dlqTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobs.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE gebweb_jobs (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, payload_json TEXT NOT NULL,
		run_at TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL,
		locked_at TEXT, locked_by TEXT, created_at TEXT NOT NULL, last_error TEXT,
		priority TEXT NOT NULL DEFAULT 'default', dedupe_key TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	seed := func(id, name, status string, attempts int, lastErr string) {
		_, err := db.Exec(
			"INSERT INTO gebweb_jobs (id, name, payload_json, run_at, attempts, status, created_at, last_error) VALUES (?, ?, '{}', '2026-01-01T00:00:00Z', ?, ?, '2026-01-01T00:00:00Z', ?)",
			id, name, attempts, status, lastErr)
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("j1", "email", "failed", 5, "boom")
	seed("j2", "resize", "failed", 3, "kaboom")
	seed("p1", "email", "pending", 0, "")
	seed("c1", "email", "completed", 1, "")
	return db, path
}

func countByStatus(t *testing.T, db *sql.DB, status string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM gebweb_jobs WHERE status = ?", status).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", status, err)
	}
	return n
}

func TestDlqListShowsOnlyFailed(t *testing.T) {
	db, _ := dlqTestDB(t)
	var sb strings.Builder
	if err := dlqList(db, &sb); err != nil {
		t.Fatalf("dlqList: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"j1", "j2", "email", "resize", "boom", "kaboom"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "p1") || strings.Contains(out, "c1") {
		t.Errorf("list leaked non-failed rows:\n%s", out)
	}
}

func TestDlqListEmpty(t *testing.T) {
	db, _ := dlqTestDB(t)
	if _, err := db.Exec("DELETE FROM gebweb_jobs WHERE status = 'failed'"); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := dlqList(db, &sb); err != nil {
		t.Fatalf("dlqList: %v", err)
	}
	if !strings.Contains(sb.String(), "no failed jobs") {
		t.Errorf("expected empty notice, got:\n%s", sb.String())
	}
}

func TestDlqRetryByID(t *testing.T) {
	db, _ := dlqTestDB(t)
	n, err := dlqRetry(db, "sqlite", []string{"j1"}, false)
	if err != nil {
		t.Fatalf("dlqRetry: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 re-queued, got %d", n)
	}
	var status string
	var attempts int
	var runAt string
	var lastErr sql.NullString
	if err := db.QueryRow("SELECT status, attempts, run_at, last_error FROM gebweb_jobs WHERE id = 'j1'").Scan(&status, &attempts, &runAt, &lastErr); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 0 || lastErr.Valid {
		t.Errorf("retry did not reset row: status=%s attempts=%d lastErr=%v", status, attempts, lastErr)
	}
	if runAt != "1970-01-01T00:00:00Z" {
		t.Errorf("run_at not reset to epoch: %s", runAt)
	}
	if countByStatus(t, db, "failed") != 1 {
		t.Errorf("expected j2 still failed")
	}
}

func TestDlqRetryAll(t *testing.T) {
	db, _ := dlqTestDB(t)
	n, err := dlqRetry(db, "sqlite", nil, true)
	if err != nil {
		t.Fatalf("dlqRetry: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 re-queued, got %d", n)
	}
	if countByStatus(t, db, "failed") != 0 {
		t.Errorf("failed jobs remain after retry --all")
	}
	if countByStatus(t, db, "pending") != 3 {
		t.Errorf("expected 3 pending (p1 + j1 + j2), got %d", countByStatus(t, db, "pending"))
	}
}

func TestDlqPurgeByID(t *testing.T) {
	db, _ := dlqTestDB(t)
	n, err := dlqPurge(db, "sqlite", []string{"j2"}, false)
	if err != nil {
		t.Fatalf("dlqPurge: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}
	if countByStatus(t, db, "failed") != 1 {
		t.Errorf("expected j1 still failed after purging j2")
	}
	var total int
	db.QueryRow("SELECT COUNT(*) FROM gebweb_jobs").Scan(&total)
	if total != 3 {
		t.Errorf("expected 3 rows remaining, got %d", total)
	}
}

func TestDlqPurgeAll(t *testing.T) {
	db, _ := dlqTestDB(t)
	n, err := dlqPurge(db, "sqlite", nil, true)
	if err != nil {
		t.Fatalf("dlqPurge: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 purged, got %d", n)
	}
	if countByStatus(t, db, "failed") != 0 {
		t.Errorf("failed jobs remain after purge --all")
	}
	if countByStatus(t, db, "pending") != 1 || countByStatus(t, db, "completed") != 1 {
		t.Errorf("purge --all touched non-failed rows")
	}
}

func TestDlqMutateRequiresTarget(t *testing.T) {
	db, _ := dlqTestDB(t)
	if _, err := dlqRetry(db, "sqlite", nil, false); err == nil {
		t.Error("expected error when neither ids nor --all given")
	}
	if countByStatus(t, db, "failed") != 2 {
		t.Error("no-op call should not have changed rows")
	}
}

// TestRunDlqEndToEnd covers the runWorker dispatch and driver detection, not just the SQL helpers.
func TestRunDlqEndToEnd(t *testing.T) {
	_, path := dlqTestDB(t)
	prev, had := os.LookupEnv("DATABASE_URL")
	t.Cleanup(func() {
		if had {
			os.Setenv("DATABASE_URL", prev)
		} else {
			os.Unsetenv("DATABASE_URL")
		}
	})
	os.Setenv("DATABASE_URL", "sqlite://"+path)

	if code := runWorker([]string{"dlq", "retry", "--all"}); code != 0 {
		t.Fatalf("runWorker dlq retry exit code: %d", code)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if countByStatus(t, db, "failed") != 0 {
		t.Errorf("dlq retry --all via runWorker left failed jobs")
	}
}
