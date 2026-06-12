package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestGenerateDockerPerDB checks that each DB choice produces a valid Dockerfile
// and compose.yaml with the right service shape.
func TestGenerateDockerPerDB(t *testing.T) {
	cases := []struct {
		db          string
		wantImage   string // expected db image, "" for sqlite (no db service)
		wantVolume  string
		wantHealth  bool
	}{
		{"sqlite", "", "appdata:", false},
		{"postgres", "image: postgres:16", "dbdata:", true},
		{"pgvector", "image: pgvector/pgvector:pg16", "dbdata:", true},
		{"mysql", "image: mysql:8", "dbdata:", true},
	}
	for _, tc := range cases {
		t.Run(tc.db, func(t *testing.T) {
			dir := t.TempDir()
			chdir(t, dir)
			if err := generateDocker(dockerOptions{db: tc.db, port: 9000, binary: "build/app"}); err != nil {
				t.Fatalf("generateDocker: %v", err)
			}

			df := readFile(t, filepath.Join(dir, "Dockerfile"))
			if !strings.Contains(df, "FROM gcr.io/distroless/base-debian12") {
				t.Errorf("Dockerfile missing distroless base:\n%s", df)
			}
			if !strings.Contains(df, "COPY build/app /app") {
				t.Errorf("Dockerfile missing binary COPY:\n%s", df)
			}
			if !strings.Contains(df, "EXPOSE 9000") || !strings.Contains(df, "ENV GEBWEB_PORT=9000") {
				t.Errorf("Dockerfile missing port wiring:\n%s", df)
			}

			cm := readFile(t, filepath.Join(dir, "compose.yaml"))
			if !strings.Contains(cm, "${GEBWEB_PORT:-9000}:${GEBWEB_PORT:-9000}") {
				t.Errorf("compose missing port mapping:\n%s", cm)
			}
			if !strings.Contains(cm, "- .env") {
				t.Errorf("compose missing env_file:\n%s", cm)
			}
			if !strings.Contains(cm, tc.wantVolume) {
				t.Errorf("compose missing volume %q:\n%s", tc.wantVolume, cm)
			}
			if tc.wantImage == "" {
				if strings.Contains(cm, "  db:") {
					t.Errorf("sqlite should have no db service:\n%s", cm)
				}
			} else {
				if !strings.Contains(cm, tc.wantImage) {
					t.Errorf("compose missing %q:\n%s", tc.wantImage, cm)
				}
				if !strings.Contains(cm, "condition: service_healthy") {
					t.Errorf("compose missing depends_on healthcheck gate:\n%s", cm)
				}
			}
			if tc.wantHealth && !strings.Contains(cm, "healthcheck:") {
				t.Errorf("compose missing healthcheck:\n%s", cm)
			}
		})
	}
}

// TestGenerateDockerIdempotent confirms existing files are preserved without
// --force and overwritten with it.
func TestGenerateDockerIdempotent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("USER EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := generateDocker(dockerOptions{db: "sqlite", port: 8080, binary: "build/app"}); err != nil {
		t.Fatalf("generateDocker: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "Dockerfile")); got != "USER EDIT" {
		t.Errorf("Dockerfile clobbered without --force: %q", got)
	}

	if err := generateDocker(dockerOptions{db: "sqlite", port: 8080, binary: "build/app", force: true}); err != nil {
		t.Fatalf("generateDocker --force: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "Dockerfile")); got == "USER EDIT" {
		t.Error("Dockerfile not overwritten with --force")
	}
}

func TestGenerateDockerRejectsUnknownDB(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := generateDocker(dockerOptions{db: "mongo", port: 8080, binary: "build/app"}); err == nil {
		t.Error("expected error for unknown --db")
	}
}
