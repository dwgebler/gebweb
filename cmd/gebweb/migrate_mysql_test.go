package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestMigrateMySQL_UpAndDownTrackSchemaChanges(t *testing.T) {
	dsn := os.Getenv("GEBWEB_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set GEBWEB_TEST_MYSQL_DSN to test MySQL migrations")
	}
	driver, connection, err := splitDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if driver != "mysql" {
		t.Fatal("GEBWEB_TEST_MYSQL_DSN must use mysql://")
	}
	admin, err := sql.Open(driver, connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := fmt.Sprintf("gebweb_migrations_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + schema); err != nil {
			t.Errorf("clean up database: %v", err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + schema
	t.Setenv("DATABASE_URL", u.String())
	dir := t.TempDir()
	chdir(t, dir)
	writeMigration(t, dir, "20260101000000", "widgets",
		"CREATE TABLE widgets (id INTEGER PRIMARY KEY)", "DROP TABLE widgets")

	up := runMigrate([]string{"up"})
	again := runMigrate([]string{"up"})

	if up != 0 || again != 0 {
		t.Fatalf("up exit codes: first=%d second=%d", up, again)
	}
	conn, err := openMigrationDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one applied migration, got %d", count)
	}
	if _, err := conn.Exec("INSERT INTO widgets VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	down := runMigrate([]string{"down"})

	if down != 0 {
		t.Fatalf("down exit code: %d", down)
	}
	if err := conn.QueryRow("SELECT COUNT(*) FROM " + schemaTable).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no applied migrations after down, got %d", count)
	}
	if _, err := conn.Exec("SELECT * FROM widgets"); err == nil {
		t.Fatal("rolled-back table still exists")
	}
}
