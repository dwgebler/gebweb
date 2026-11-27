package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

func postgresMigrationProject(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GEBWEB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GEBWEB_TEST_POSTGRES_DSN to test PostgreSQL migrations")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("gebweb_migrations_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("clean up schema: %v", err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	t.Setenv("DATABASE_URL", u.String())
	chdir(t, t.TempDir())
	conn, err := openMigrationDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestMigratePostgres_UpRecordsVersionAndSkipsReapplication(t *testing.T) {
	conn := postgresMigrationProject(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeMigration(t, dir, "20260101000000", "widgets",
		"CREATE TABLE widgets (id INTEGER PRIMARY KEY)", "DROP TABLE widgets")

	first := runMigrate([]string{"up"})
	second := runMigrate([]string{"up"})

	if first != 0 || second != 0 {
		t.Fatalf("up exit codes: first=%d second=%d", first, second)
	}
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one applied migration, got %d", count)
	}
	if _, err := conn.Exec("INSERT INTO widgets VALUES (1)"); err != nil {
		t.Fatalf("migration schema unavailable: %v", err)
	}
}

func TestMigratePostgres_DownRemovesVersionAndSchema(t *testing.T) {
	conn := postgresMigrationProject(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeMigration(t, dir, "20260101000000", "widgets",
		"CREATE TABLE widgets (id INTEGER PRIMARY KEY)", "DROP TABLE widgets")
	if err := ensureSchemaTable(conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO "+schemaTable+" VALUES ($1, $2)", "20260101000000", "2026-01-01"); err != nil {
		t.Fatal(err)
	}

	code := runMigrate([]string{"down"})

	if code != 0 {
		t.Fatalf("down exit code: %d", code)
	}
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no applied migrations, got %d", count)
	}
	if _, err := conn.Exec("SELECT * FROM widgets"); err == nil {
		t.Fatal("rolled-back table still exists")
	}
}
