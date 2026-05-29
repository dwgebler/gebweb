package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrateInDir cds into dir, sets DATABASE_URL to a temp sqlite
// file, runs runMigrate, and restores cwd / env afterwards.
func migrateInDir(t *testing.T, dir string, args ...string) int {
	t.Helper()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	prev, hadPrev := os.LookupEnv("DATABASE_URL")
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv("DATABASE_URL", prev)
		} else {
			os.Unsetenv("DATABASE_URL")
		}
	})
	os.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "test.db"))
	return runMigrate(args)
}

func writeMigration(t *testing.T, dir, version, name, up, down string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	body := "-- +gebweb up\n" + up + "\n\n-- +gebweb down\n" + down + "\n"
	path := filepath.Join(dir, "migrations", version+"_"+name+".sql")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestMigrateCreate(t *testing.T) {
	dir := t.TempDir()
	if code := migrateInDir(t, dir, "create", "Add Users Table"); code != 0 {
		t.Fatalf("create exit code: %d", code)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one migration file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, "_add_users_table.sql") {
		t.Errorf("expected sanitised name, got %s", name)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "migrations", name))
	if !strings.Contains(string(body), "-- +gebweb up") || !strings.Contains(string(body), "-- +gebweb down") {
		t.Errorf("template missing up/down markers: %s", string(body))
	}
}

func TestMigrateUpDownStatus(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "20260101000000", "users",
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
		"DROP TABLE users",
	)
	writeMigration(t, dir, "20260101000100", "products",
		"CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
		"DROP TABLE products",
	)

	if code := migrateInDir(t, dir, "up"); code != 0 {
		t.Fatalf("up exit code: %d", code)
	}

	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO users (name) VALUES ('alice')"); err != nil {
		t.Fatalf("users table missing after up: %v", err)
	}
	if _, err := db.Exec("INSERT INTO products (name) VALUES ('widget')"); err != nil {
		t.Fatalf("products table missing after up: %v", err)
	}
	var applied int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&applied); err != nil {
		t.Fatalf("count applied: %v", err)
	}
	if applied != 2 {
		t.Errorf("expected 2 applied migrations, got %d", applied)
	}

	if code := migrateInDir(t, dir, "down", "--steps", "1"); code != 0 {
		t.Fatalf("down exit code: %d", code)
	}
	if _, err := db.Exec("INSERT INTO products (name) VALUES ('rolled-back')"); err == nil {
		t.Errorf("products table should be dropped after down --steps 1")
	}
	if _, err := db.Exec("INSERT INTO users (name) VALUES ('still-here')"); err != nil {
		t.Errorf("users table should remain after down --steps 1: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&applied); err != nil {
		t.Fatalf("count applied after down: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 applied migration after down, got %d", applied)
	}

	if code := migrateInDir(t, dir, "status"); code != 0 {
		t.Fatalf("status exit code: %d", code)
	}
}

func TestMigrateUpIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "20260101000000", "noop",
		"CREATE TABLE noop (id INTEGER PRIMARY KEY)",
		"DROP TABLE noop",
	)
	if code := migrateInDir(t, dir, "up"); code != 0 {
		t.Fatalf("first up: %d", code)
	}
	// Second up against the same migrations should be a no-op
	// rather than re-applying and breaking with "table exists".
	if code := migrateInDir(t, dir, "up"); code != 0 {
		t.Fatalf("second up should be no-op, got exit %d", code)
	}
}

func TestSplitDSN(t *testing.T) {
	cases := []struct {
		in, driver, conn string
		wantErr          bool
	}{
		{"sqlite:///tmp/x.db", "sqlite", "/tmp/x.db", false},
		{"sqlite:./local.db", "sqlite", "./local.db", false},
		{"postgres://u:p@host:5432/db", "pgx", "postgres://u:p@host:5432/db", false},
		{"postgresql://u:p@host:5432/db", "pgx", "postgresql://u:p@host:5432/db", false},
		{"mysql://u:p@host:3306/db", "mysql", "u:p@tcp(host:3306)/db", false},
		{"oracle://u:p@host/db", "", "", true},
	}
	for _, c := range cases {
		d, conn, err := splitDSN(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("splitDSN(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitDSN(%q): unexpected error %v", c.in, err)
			continue
		}
		if d != c.driver || conn != c.conn {
			t.Errorf("splitDSN(%q) = (%q, %q), want (%q, %q)", c.in, d, conn, c.driver, c.conn)
		}
	}
}

func TestSplitUpDown(t *testing.T) {
	content := `-- some preamble
-- +gebweb up
CREATE TABLE x (id INT);
INSERT INTO x VALUES (1);

-- +gebweb down
DROP TABLE x;
`
	up, down := splitUpDown(content)
	if !strings.Contains(up, "CREATE TABLE x") || !strings.Contains(up, "INSERT INTO x") {
		t.Errorf("up block missing content: %q", up)
	}
	if strings.Contains(up, "DROP TABLE") {
		t.Errorf("up block bled into down: %q", up)
	}
	if !strings.Contains(down, "DROP TABLE x") {
		t.Errorf("down block missing content: %q", down)
	}
}
