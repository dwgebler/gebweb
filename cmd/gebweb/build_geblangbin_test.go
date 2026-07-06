package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldMinimalBuildProject satisfies runBuild's manifest/asset checks with no real assets.
func scaffoldMinimalBuildProject(t *testing.T, dir string) {
	t.Helper()
	chdir(t, dir)
	writeFile(t, dir, "geblang.yaml", "name: t\nversion: 0.1.0\nsource: src\n")
}

// newFakeGeblang stands in for the real compiler, recording its invocation by creating recordFile.
func newFakeGeblang(t *testing.T, path, recordFile string) {
	t.Helper()
	script := "#!/bin/sh\n> " + recordFile + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fakeGeblangRan(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// captureBuildStderr runs fn with os.Stderr redirected and returns (exit code, captured stderr).
func captureBuildStderr(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	code := fn()
	w.Close()
	os.Stderr = orig
	data, _ := io.ReadAll(r)
	return code, string(data)
}

func TestRunBuildGeblangFlagTakesPrecedenceOverEnv(t *testing.T) {
	dir := t.TempDir()
	scaffoldMinimalBuildProject(t, dir)

	flagBin := filepath.Join(dir, "flag-geblang")
	envBin := filepath.Join(dir, "env-geblang")
	flagRecord := filepath.Join(dir, "flag-ran")
	envRecord := filepath.Join(dir, "env-ran")
	newFakeGeblang(t, flagBin, flagRecord)
	newFakeGeblang(t, envBin, envRecord)
	t.Setenv("GEBLANG_BIN", envBin)

	if code := runBuild([]string{"--geblang", flagBin, "--no-swagger"}); code != 0 {
		t.Fatalf("runBuild exit code: %d", code)
	}
	if !fakeGeblangRan(flagRecord) {
		t.Error("expected --geblang binary to run")
	}
	if fakeGeblangRan(envRecord) {
		t.Error("GEBLANG_BIN binary should not run when --geblang is set")
	}
}

func TestRunBuildFallsBackToGeblangBinEnv(t *testing.T) {
	dir := t.TempDir()
	scaffoldMinimalBuildProject(t, dir)

	envBin := filepath.Join(dir, "env-geblang")
	envRecord := filepath.Join(dir, "env-ran")
	newFakeGeblang(t, envBin, envRecord)
	t.Setenv("GEBLANG_BIN", envBin)

	if code := runBuild([]string{"--no-swagger"}); code != 0 {
		t.Fatalf("runBuild exit code: %d", code)
	}
	if !fakeGeblangRan(envRecord) {
		t.Error("expected GEBLANG_BIN binary to run")
	}
}

func TestRunBuildDefaultsToGeblangOnPath(t *testing.T) {
	dir := t.TempDir()
	scaffoldMinimalBuildProject(t, dir)

	binDir := t.TempDir()
	record := filepath.Join(dir, "path-ran")
	newFakeGeblang(t, filepath.Join(binDir, "geblang"), record)
	t.Setenv("GEBLANG_BIN", "")
	t.Setenv("PATH", binDir)

	if code := runBuild([]string{"--no-swagger"}); code != 0 {
		t.Fatalf("runBuild exit code: %d", code)
	}
	if !fakeGeblangRan(record) {
		t.Error("expected geblang resolved from PATH to run")
	}
}

func TestRunBuildNonexistentGeblangFailsClearly(t *testing.T) {
	dir := t.TempDir()
	scaffoldMinimalBuildProject(t, dir)

	missing := filepath.Join(dir, "does-not-exist-geblang")
	code, stderr := captureBuildStderr(t, func() int {
		return runBuild([]string{"--geblang", missing, "--no-swagger"})
	})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "gebweb build:") || !strings.Contains(stderr, "no such file") {
		t.Errorf("expected a clear missing-binary error, got: %s", stderr)
	}
}
