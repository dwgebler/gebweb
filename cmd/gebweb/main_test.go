package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNewCreatesScaffold(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runNew([]string{"myapp"}); code != 0 {
		t.Fatalf("runNew exit code: %d", code)
	}
	for _, rel := range []string{
		"myapp/geblang.yaml",
		"myapp/src/main.gb",
		"myapp/src/main_test.gb",
		"myapp/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected file %s: %v", rel, err)
		}
	}
	main, err := os.ReadFile(filepath.Join(dir, "myapp/src/main.gb"))
	if err != nil {
		t.Fatalf("read main.gb: %v", err)
	}
	if !strings.Contains(string(main), `class HelloController`) {
		t.Errorf("main.gb missing HelloController scaffold")
	}
}

func TestRunGenerateController(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runGenerate([]string{"controller", "Product"}); code != 0 {
		t.Fatalf("runGenerate exit code: %d", code)
	}
	body, err := os.ReadFile(filepath.Join(dir, "src/product_controller.gb"))
	if err != nil {
		t.Fatalf("read controller: %v", err)
	}
	if !strings.Contains(string(body), `class ProductController`) {
		t.Errorf("controller missing class name: %s", string(body))
	}
	if !strings.Contains(string(body), `"/products"`) {
		t.Errorf("controller missing route path: %s", string(body))
	}
}

func TestRunGenerateRejectsUnknownKind(t *testing.T) {
	if code := runGenerate([]string{"widget", "X"}); code != 2 {
		t.Errorf("expected exit 2 for unknown kind, got %d", code)
	}
}

func TestRunGenerateResource(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runGenerate([]string{"resource", "Article"}); code != 0 {
		t.Fatalf("runGenerate resource exit code: %d", code)
	}
	for _, rel := range []string{
		"src/article_dto.gb",
		"src/article_repository.gb",
		"src/article_controller.gb",
		"tests/article_resource_test.gb",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected file %s: %v", rel, err)
		}
	}
	ctrl, err := os.ReadFile(filepath.Join(dir, "src/article_controller.gb"))
	if err != nil {
		t.Fatalf("read controller: %v", err)
	}
	if !strings.Contains(string(ctrl), `@ApiResource("/articles")`) {
		t.Errorf("resource controller missing @ApiResource: %s", string(ctrl))
	}
	if !strings.Contains(string(ctrl), `class ArticleController`) {
		t.Errorf("resource controller missing class name: %s", string(ctrl))
	}
}

func TestRunWorkerRequiresManifest(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runWorker(nil); code != 1 {
		t.Errorf("expected exit 1 without manifest, got %d", code)
	}
}

func TestHasHelpFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"-h"},
		{"--entry", "foo", "--help"},
		{"--job", "email", "-h"},
	} {
		if !hasHelpFlag(args) {
			t.Errorf("expected hasHelpFlag(%v) = true", args)
		}
	}
	for _, args := range [][]string{
		nil,
		{},
		{"--entry", "foo"},
	} {
		if hasHelpFlag(args) {
			t.Errorf("expected hasHelpFlag(%v) = false", args)
		}
	}
}

func TestRunWorkerHelpShortCircuits(t *testing.T) {
	/* --help must work even with no manifest in cwd. */
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runWorker([]string{"--help"}); code != 0 {
		t.Errorf("worker --help should exit 0, got %d", code)
	}
}

func TestRunWorkerRejectsUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile("geblang.yaml", []byte("name: t\nversion: 0.1.0\nsource: src\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if code := runWorker([]string{"--nonsense"}); code != 2 {
		t.Errorf("expected exit 2 on unknown flag, got %d", code)
	}
}
