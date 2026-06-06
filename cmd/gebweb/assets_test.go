package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}

// TestBuildAssetsCompilesAndStages exercises the JS/TS + CSS esbuild path and
// HTML template minification, and checks the --resource specs handed to
// geblang build.
func TestBuildAssetsCompilesAndStages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "assets/app.ts", "const greeting: string = \"hi\";\nconsole.log(greeting);\n")
	writeFile(t, root, "assets/app.css", "body {\n  color: red;\n}\n")
	writeFile(t, root, "templates/page.html", "<h1>   {{ title }}   </h1>\n\n<p>hello</p>\n")
	writeFile(t, root, "public/logo.svg", "<svg></svg>")
	chdir(t, root)

	cfg := &assetsConfig{
		SourceDir:    "assets",
		OutDir:       "build/assets",
		EntryPoints:  []string{"app.ts", "app.css"},
		TemplatesDir: "templates",
		PublicDir:    "public",
	}
	resources, err := buildAssets(cfg, assetBuildOptions{minify: true})
	if err != nil {
		t.Fatalf("buildAssets: %v", err)
	}

	js, err := os.ReadFile(filepath.Join(root, "build/assets/app.js"))
	if err != nil {
		t.Fatalf("compiled js missing: %v", err)
	}
	if strings.Contains(string(js), ": string") {
		t.Error("TypeScript types not stripped by esbuild")
	}
	css, err := os.ReadFile(filepath.Join(root, "build/assets/app.css"))
	if err != nil {
		t.Fatalf("compiled css missing: %v", err)
	}
	if strings.Contains(string(css), "  ") {
		t.Errorf("css not minified: %q", css)
	}

	staged, err := os.ReadFile(filepath.Join(root, stageDir, "templates", "page.html"))
	if err != nil {
		t.Fatalf("staged template missing: %v", err)
	}
	if strings.Contains(string(staged), "   {{ title }}   ") {
		t.Errorf("template whitespace not minified: %q", staged)
	}
	if !strings.Contains(string(staged), "{{ title }}") {
		t.Errorf("template tag not preserved: %q", staged)
	}

	wantResources := []string{
		"build/assets",
		filepath.ToSlash(filepath.Join(stageDir, "templates")) + "=templates",
		"public",
	}
	if len(resources) != len(wantResources) {
		t.Fatalf("resources = %v, want %v", resources, wantResources)
	}
	for i, r := range wantResources {
		if filepath.ToSlash(resources[i]) != r {
			t.Errorf("resource[%d] = %q, want %q", i, resources[i], r)
		}
	}

	// Source tree must be untouched.
	srcTpl, _ := os.ReadFile(filepath.Join(root, "templates/page.html"))
	if !strings.Contains(string(srcTpl), "   {{ title }}   ") {
		t.Error("source template was modified by staging")
	}
}

// TestBuildAssetsSassWithoutBinary covers the dart-sass-absent contract: an
// error by default, skipped under --no-sass.
func TestBuildAssetsSassWithoutBinary(t *testing.T) {
	if findSass() != "" {
		t.Skip("dart-sass present; this test covers its absence")
	}
	root := t.TempDir()
	writeFile(t, root, "assets/app.scss", "$c: red;\nbody { color: $c; }\n")
	chdir(t, root)

	cfg := &assetsConfig{SourceDir: "assets", OutDir: "build/assets", EntryPoints: []string{"app.scss"}}

	if _, err := buildAssets(cfg, assetBuildOptions{minify: true}); err == nil {
		t.Error("expected error when dart-sass is missing")
	} else if !strings.Contains(err.Error(), "--no-sass") {
		t.Errorf("error should mention --no-sass: %v", err)
	}

	if _, err := buildAssets(cfg, assetBuildOptions{minify: true, noSass: true}); err != nil {
		t.Errorf("--no-sass should skip SASS, got: %v", err)
	}
}

// TestReadAssetsConfig parses the assets: block and applies dir defaults.
func TestReadAssetsConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "geblang.yaml", "name: app\nassets:\n  sourceDir: src/assets\n  outDir: build/assets\n  entryPoints:\n    - app.ts\n")
	cfg, err := readAssetsConfig(filepath.Join(root, "geblang.yaml"))
	if err != nil {
		t.Fatalf("readAssetsConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected an assets config")
	}
	if cfg.SourceDir != "src/assets" || cfg.OutDir != "build/assets" {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
	if cfg.TemplatesDir != "templates" || cfg.PublicDir != "public" {
		t.Errorf("dir defaults not applied: %+v", cfg)
	}

	writeFile(t, root, "noassets.yaml", "name: app\nsource: src\n")
	cfg2, err := readAssetsConfig(filepath.Join(root, "noassets.yaml"))
	if err != nil {
		t.Fatalf("readAssetsConfig: %v", err)
	}
	if cfg2 != nil {
		t.Errorf("expected nil config when no assets block, got %+v", cfg2)
	}
}
