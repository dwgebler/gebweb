package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldToolchain returns the geblang binary and framework directory to run generated projects against, or skips.
func scaffoldToolchain(t *testing.T) (string, string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wd, "..", "..")
	framework := filepath.Join(root, "gebweb")
	if _, err := os.Stat(filepath.Join(framework, "src", "gebweb.gb")); err != nil {
		framework = root
	}
	if _, err := os.Stat(filepath.Join(framework, "src", "gebweb.gb")); err != nil {
		t.Skip("framework source not found")
	}
	bin := os.Getenv("GEBLANG_BIN")
	if bin == "" {
		bin = filepath.Join(root, "geblang")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("no geblang binary; set GEBLANG_BIN")
	}
	return bin, framework
}

func TestScaffoldedProjectsPassTheirOwnTests(t *testing.T) {
	bin, framework := scaffoldToolchain(t)
	for _, typ := range []string{"app", "api"} {
		t.Run(typ, func(t *testing.T) {
			dir := t.TempDir()
			chdir(t, dir)
			if code := runNew([]string{"proj", "--type", typ, "--db", "sqlite", "--yes"}); code != 0 {
				t.Fatalf("runNew exit code: %d", code)
			}
			project := filepath.Join(dir, "proj")
			manifestPath := filepath.Join(project, "geblang.yaml")
			manifest := readFile(t, manifestPath)
			pinned := "    git: https://github.com/dwgebler/gebweb\n    version: v" + version + "\n"
			if !strings.Contains(manifest, pinned) {
				t.Fatalf("manifest does not pin gebweb:\n%s", manifest)
			}
			local := strings.Replace(manifest, pinned, "    path: "+framework+"\n", 1)
			if err := os.WriteFile(manifestPath, []byte(local), 0o644); err != nil {
				t.Fatal(err)
			}
			sources, err := filepath.Glob(filepath.Join(project, "src", "*.gb"))
			if err != nil || len(sources) == 0 {
				t.Fatalf("no generated sources: %v", err)
			}
			for _, src := range sources {
				body, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				fmtCmd := exec.Command(bin, "fmt", "--stdin")
				fmtCmd.Stdin = strings.NewReader(string(body))
				formatted, err := fmtCmd.Output()
				if err != nil || string(formatted) != string(body) {
					t.Errorf("%s is not geblang fmt formatted (%v); want:\n%s", filepath.Base(src), err, formatted)
				}
			}
			for _, flag := range []string{"--help", "--version"} {
				cmd := exec.Command(bin, "run", "src/main.gb", flag)
				cmd.Dir = project
				out, err := cmd.CombinedOutput()
				want := map[string]string{"--help": "proj server options", "--version": "proj 0.1.0\n  geblang "}[flag]
				if err != nil || !strings.Contains(string(out), want) {
					t.Fatalf("main %s: %v\n%s", flag, err, out)
				}
				if _, err := os.Stat(filepath.Join(project, "app.db")); err == nil {
					t.Fatalf("main %s opened the database before answering", flag)
				}
			}
			for _, runtime := range []string{"vm", "evaluator"} {
				cmd := exec.Command(bin, "test", "--runtime="+runtime, "src/")
				cmd.Dir = project
				out, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(out), "failed=0") {
					t.Fatalf("%s tests failed: %v\n%s", runtime, err, out)
				}
			}
		})
	}
}
