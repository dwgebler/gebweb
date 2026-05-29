package main

// Migration runner for `gebweb migrate`. Reads versioned SQL files
// from ./migrations/ and tracks applied versions in a
// gebweb_schema_migrations table. Uses database/sql directly with
// the existing sqlite / pgx / mysql drivers so the CLI doesn't have
// to shell out to a Geblang interpreter just to run SQL.

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const migrationsDir = "migrations"
const schemaTable = "gebweb_schema_migrations"

// migrationFile pairs the version timestamp prefix with the parsed
// up/down SQL blocks. up/down are extracted from headers of the form
//
//	-- +gebweb up
//	CREATE TABLE ...
//	-- +gebweb down
//	DROP TABLE ...
type migrationFile struct {
	version string
	name    string
	path    string
	upSQL   string
	downSQL string
}

func runMigrate(args []string) int {
	if hasHelpFlag(args) {
		printMigrateHelp(os.Stdout)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gebweb migrate <create|up|down|status> [args]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return runMigrateCreate(rest)
	case "up":
		return runMigrateUp(rest)
	case "down":
		return runMigrateDown(rest)
	case "status":
		return runMigrateStatus(rest)
	default:
		fmt.Fprintf(os.Stderr, "gebweb migrate: unknown subcommand %q (create|up|down|status)\n", sub)
		return 2
	}
}

func runMigrateCreate(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gebweb migrate create <name>")
		return 2
	}
	name := sanitiseMigrationName(args[0])
	if name == "" {
		fmt.Fprintln(os.Stderr, "gebweb migrate create: name must be non-empty after sanitisation")
		return 2
	}
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate create: %v\n", err)
		return 1
	}
	ts := time.Now().UTC().Format("20060102150405")
	path := filepath.Join(migrationsDir, ts+"_"+name+".sql")
	content := "-- +gebweb up\n-- write the forward migration here\n\n-- +gebweb down\n-- write the rollback here\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate create: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s\n", path)
	return 0
}

func runMigrateUp(args []string) int {
	target := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--target" && i+1 < len(args) {
			target = args[i+1]
			i++
		}
	}
	db, err := openMigrationDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate up: %v\n", err)
		return 1
	}
	defer db.Close()
	if err := ensureSchemaTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate up: %v\n", err)
		return 1
	}
	files, err := loadMigrationFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate up: %v\n", err)
		return 1
	}
	applied, err := loadAppliedVersions(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate up: %v\n", err)
		return 1
	}
	pending := 0
	for _, m := range files {
		if applied[m.version] {
			continue
		}
		if target != "" && m.version > target {
			break
		}
		if m.upSQL == "" {
			fmt.Fprintf(os.Stderr, "gebweb migrate up: %s has no `-- +gebweb up` block\n", m.path)
			return 1
		}
		if err := applyMigration(db, m); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb migrate up: %s: %v\n", m.version, err)
			return 1
		}
		fmt.Printf("applied %s_%s\n", m.version, m.name)
		pending++
	}
	if pending == 0 {
		fmt.Println("no pending migrations")
	}
	return 0
}

func runMigrateDown(args []string) int {
	steps := 1
	for i := 0; i < len(args); i++ {
		if args[i] == "--steps" && i+1 < len(args) {
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				fmt.Fprintln(os.Stderr, "gebweb migrate down: --steps must be a positive integer")
				return 2
			}
			steps = n
			i++
		}
	}
	db, err := openMigrationDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate down: %v\n", err)
		return 1
	}
	defer db.Close()
	if err := ensureSchemaTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate down: %v\n", err)
		return 1
	}
	files, err := loadMigrationFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate down: %v\n", err)
		return 1
	}
	applied, err := loadAppliedVersions(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate down: %v\n", err)
		return 1
	}
	// Iterate in reverse order, rolling back applied migrations.
	rolled := 0
	for i := len(files) - 1; i >= 0 && rolled < steps; i-- {
		m := files[i]
		if !applied[m.version] {
			continue
		}
		if m.downSQL == "" {
			fmt.Fprintf(os.Stderr, "gebweb migrate down: %s has no `-- +gebweb down` block\n", m.path)
			return 1
		}
		if err := rollbackMigration(db, m); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb migrate down: %s: %v\n", m.version, err)
			return 1
		}
		fmt.Printf("rolled back %s_%s\n", m.version, m.name)
		rolled++
	}
	if rolled == 0 {
		fmt.Println("no migrations to roll back")
	}
	return 0
}

func runMigrateStatus(_ []string) int {
	db, err := openMigrationDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate status: %v\n", err)
		return 1
	}
	defer db.Close()
	if err := ensureSchemaTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate status: %v\n", err)
		return 1
	}
	files, err := loadMigrationFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate status: %v\n", err)
		return 1
	}
	applied, err := loadAppliedVersions(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb migrate status: %v\n", err)
		return 1
	}
	for _, m := range files {
		mark := "pending"
		if applied[m.version] {
			mark = "applied"
		}
		fmt.Printf("%-8s  %s_%s\n", mark, m.version, m.name)
	}
	if len(files) == 0 {
		fmt.Println("no migration files in ./migrations/")
	}
	return 0
}

// openMigrationDB opens the database from $DATABASE_URL. The URL
// scheme picks the driver: sqlite, postgres, mysql.
func openMigrationDB() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}
	driver, connStr, err := splitDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return db, nil
}

// splitDSN translates a friendly DATABASE_URL into (driverName,
// driverConnString). For sqlite the connection string is the file
// path; for postgres / mysql the URL is passed through to the
// driver.
func splitDSN(dsn string) (string, string, error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		return "sqlite", strings.TrimPrefix(dsn, "sqlite://"), nil
	case strings.HasPrefix(dsn, "sqlite:"):
		return "sqlite", strings.TrimPrefix(dsn, "sqlite:"), nil
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		return "pgx", dsn, nil
	case strings.HasPrefix(dsn, "mysql://"):
		// go-sql-driver/mysql doesn't accept the mysql:// scheme; convert.
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", err
		}
		userInfo := ""
		if u.User != nil {
			userInfo = u.User.String() + "@"
		}
		conn := fmt.Sprintf("%stcp(%s)%s", userInfo, u.Host, u.Path)
		if u.RawQuery != "" {
			conn += "?" + u.RawQuery
		}
		return "mysql", conn, nil
	default:
		return "", "", fmt.Errorf("unsupported DATABASE_URL scheme; expected sqlite://, postgres://, or mysql://")
	}
}

func ensureSchemaTable(db *sql.DB) error {
	_, err := db.Exec(fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)",
		schemaTable,
	))
	return err
}

func loadAppliedVersions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM " + schemaTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func loadMigrationFiles() ([]migrationFile, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m, err := parseMigrationFile(filepath.Join(migrationsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func parseMigrationFile(path string) (migrationFile, error) {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, ".sql")
	parts := strings.SplitN(stem, "_", 2)
	if len(parts) != 2 || parts[0] == "" {
		return migrationFile{}, fmt.Errorf("malformed migration filename %q (expected <version>_<name>.sql)", base)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return migrationFile{}, err
	}
	up, down := splitUpDown(string(content))
	return migrationFile{version: parts[0], name: parts[1], path: path, upSQL: up, downSQL: down}, nil
}

func splitUpDown(content string) (string, string) {
	var up, down strings.Builder
	target := (*strings.Builder)(nil)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- +gebweb up") {
			target = &up
			continue
		}
		if strings.HasPrefix(trimmed, "-- +gebweb down") {
			target = &down
			continue
		}
		if target != nil {
			target.WriteString(line)
			target.WriteString("\n")
		}
	}
	return strings.TrimSpace(up.String()), strings.TrimSpace(down.String())
}

func applyMigration(db *sql.DB, m migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(m.upSQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(
		"INSERT INTO "+schemaTable+" (version, applied_at) VALUES (?, ?)",
		m.version, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func rollbackMigration(db *sql.DB, m migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(m.downSQL); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM "+schemaTable+" WHERE version = ?", m.version); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// sanitiseMigrationName drops characters unsafe for filenames and
// lower-cases the result; spaces and hyphens become underscores.
func sanitiseMigrationName(in string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(in) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
