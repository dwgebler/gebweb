package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	doctestMarker = regexp.MustCompile(`^<!--\s*doctest:([a-z]+)\s*(.*?)\s*-->$`)
	strictMarker  = "<!-- doctest:strict -->"
)

type docExample struct {
	line       int
	kind       string
	arg        string
	code       string
	output     string
	hasOutput  bool
	outputLine int
	files      map[string]string
	malformed  bool
	geblang    bool
}

type docProblem struct {
	line int
	msg  string
}

// parseDocExamples returns the gb blocks of a chapter and the marker problems that need no toolchain to detect.
func parseDocExamples(src string) ([]docExample, []docProblem) {
	lines := strings.Split(src, "\n")
	strict := strings.Contains(src, strictMarker)
	var examples []docExample
	var problems []docProblem
	files := map[string]string{}
	for i := 0; i < len(lines); i++ {
		fence := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(fence, "```") {
			continue
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		ex := docExample{line: i + 1, geblang: fence == "```gb" || fence == "```geblang"}
		ex.kind, ex.arg = markerAbove(lines, i)
		body, end := fencedBody(lines, i+1, indent)
		ex.code = body
		i = end
		if !ex.geblang && ex.kind != "file" {
			continue
		}
		if ex.kind == "run" {
			ex.output, ex.outputLine, ex.hasOutput = outputBelow(lines, end+1)
		}
		ex.files = make(map[string]string, len(files))
		for name, code := range files {
			ex.files[name] = code
		}
		if msg := markerProblem(ex, strict); msg != "" {
			ex.malformed = true
			problems = append(problems, docProblem{ex.line, msg})
		} else if ex.kind == "file" {
			files[ex.arg] = ex.code
		}
		examples = append(examples, ex)
	}
	return examples, problems
}

func markerAbove(lines []string, fence int) (string, string) {
	for j := fence - 1; j >= 0; j-- {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		if m := doctestMarker.FindStringSubmatch(t); m != nil && m[1] != "strict" {
			return m[1], m[2]
		}
		return "", ""
	}
	return "", ""
}

func fencedBody(lines []string, start int, indent string) (string, int) {
	var body []string
	i := start
	for ; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
		body = append(body, strings.TrimPrefix(lines[i], indent))
	}
	return strings.Join(body, "\n"), i
}

// outputBelow finds the ```text block after a runnable example, allowing one lead-in paragraph such as "Output:".
func outputBelow(lines []string, start int) (string, int, bool) {
	paragraphs := 0
	inParagraph := false
	for i := start; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "":
			inParagraph = false
		case t == "```text":
			body, _ := fencedBody(lines, i+1, lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))])
			return body, i + 1, true
		case strings.HasPrefix(t, "```"):
			return "", 0, false
		case !inParagraph:
			inParagraph = true
			paragraphs++
			if paragraphs > 1 {
				return "", 0, false
			}
		}
	}
	return "", 0, false
}

func markerProblem(ex docExample, strict bool) string {
	switch ex.kind {
	case "":
		if strict {
			return "gb block has no doctest marker (run, file, fragment, excerpt or skip)"
		}
	case "run":
		if !ex.hasOutput {
			return "doctest:run block is not followed by a ```text output block"
		}
	case "file":
		if ex.geblang && (!strings.HasSuffix(ex.arg, ".gb") || strings.ContainsAny(ex.arg, "/ \t")) {
			return "doctest:file needs a module file name such as notes.gb"
		}
		if !ex.geblang && (ex.arg == "" || strings.HasPrefix(ex.arg, "/") || strings.Contains(ex.arg, "..") || strings.ContainsAny(ex.arg, " \t")) {
			return "doctest:file needs a project-relative path such as templates/home.html"
		}
	case "fragment":
		if _, ok := markerArg(ex.arg, "context"); !ok {
			return "doctest:fragment needs context=<name>"
		}
	case "excerpt":
		src, ok := markerArg(ex.arg, "src")
		if !ok || !strings.Contains(src, "#") {
			return "doctest:excerpt needs src=<path>#<region>"
		}
	case "skip":
		if strings.TrimSpace(ex.arg) == "" {
			return "doctest:skip needs a reason"
		}
	default:
		return fmt.Sprintf("unknown doctest marker %q", ex.kind)
	}
	return ""
}

func markerArg(arg, key string) (string, bool) {
	for _, f := range strings.Fields(arg) {
		if v, ok := strings.CutPrefix(f, key+"="); ok && v != "" {
			return v, true
		}
	}
	return "", false
}

type docToolchain struct {
	bin       string
	framework string
	contexts  string
	excerpts  string
}

// project writes the chapter's earlier doctest:file modules beside the entry file.
func (tc docToolchain) project(t *testing.T, entry, code string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := "name: doctest\nversion: 0.1.0\nsource: src\ndependencies:\n  gebweb:\n    path: " + tc.framework + "\n"
	if err := os.WriteFile(filepath.Join(dir, "geblang.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if strings.HasSuffix(name, ".gb") && !strings.Contains(name, "/") {
			target = filepath.Join(dir, "src", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "src", entry), []byte(code+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// exampleEnv keeps the caller's GEBWEB_* settings out and makes cli/serve print routes instead of listening.
func exampleEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GEBWEB_") {
			env = append(env, kv)
		}
	}
	return append(env, "GEBWEB_PRINT_ROUTES=1")
}

func (tc docToolchain) exec(dir string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tc.bin, args...)
	cmd.Dir = dir
	cmd.Env = exampleEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (tc docToolchain) check(t *testing.T, entry, code string, files map[string]string) error {
	dir := tc.project(t, entry, code, files)
	out, errOut, err := tc.exec(dir, "check", "--strict", "src/"+entry)
	if err != nil {
		return fmt.Errorf("geblang check --strict failed:\n%s%s", out, errOut)
	}
	return nil
}

func (tc docToolchain) verifyRun(t *testing.T, ex docExample) error {
	entry, ok := markerArg(ex.arg, "file")
	if !ok {
		entry = "example.gb"
	}
	if err := tc.check(t, entry, ex.code, ex.files); err != nil {
		return err
	}
	want := strings.TrimRight(ex.output, "\n")
	for _, runtime := range [][]string{{"run"}, {"run", "--disable-vm"}} {
		dir := tc.project(t, entry, ex.code, ex.files)
		out, errOut, err := tc.exec(dir, append(runtime, "src/"+entry)...)
		name := "VM"
		if len(runtime) > 1 {
			name = "evaluator"
		}
		if err != nil {
			return fmt.Errorf("%s run failed: %v\n%s%s", name, err, out, errOut)
		}
		if got := strings.TrimRight(out, "\n"); got != want {
			return fmt.Errorf("%s output differs from the output block at line %d\n--- want\n%s\n--- got\n%s", name, ex.outputLine, want, got)
		}
	}
	return nil
}

func (tc docToolchain) verifyFragment(t *testing.T, ex docExample) error {
	name, _ := markerArg(ex.arg, "context")
	raw, err := os.ReadFile(filepath.Join(tc.contexts, name+".gb"))
	if err != nil {
		return fmt.Errorf("fragment context %q: %v", name, err)
	}
	program, ok := insertFragment(string(raw), ex.code)
	if !ok {
		return fmt.Errorf("fragment context %q has no /* doctest:fragment */ line", name)
	}
	return tc.check(t, "example.gb", program, ex.files)
}

func insertFragment(context, fragment string) (string, bool) {
	lines := strings.Split(context, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "/* doctest:fragment */" {
			continue
		}
		indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		var body []string
		for _, f := range strings.Split(fragment, "\n") {
			if strings.TrimSpace(f) == "" {
				body = append(body, "")
			} else {
				body = append(body, indent+f)
			}
		}
		out := append(append(append([]string{}, lines[:i]...), body...), lines[i+1:]...)
		return strings.Join(out, "\n"), true
	}
	return "", false
}

func (tc docToolchain) verifyExcerpt(ex docExample) error {
	src, _ := markerArg(ex.arg, "src")
	path, region, _ := strings.Cut(src, "#")
	raw, err := os.ReadFile(filepath.Join(tc.excerpts, path))
	if err != nil {
		return fmt.Errorf("excerpt source: %v", err)
	}
	want, ok := sourceRegion(string(raw), region)
	if !ok {
		return fmt.Errorf("%s has no doctest:region %s ... doctest:endregion", path, region)
	}
	if strings.TrimRight(ex.code, "\n") != want {
		return fmt.Errorf("excerpt differs from %s#%s\n--- source\n%s", path, region, want)
	}
	return nil
}

// sourceRegion returns the dedented lines between `/* doctest:region NAME */` and `/* doctest:endregion */`.
func sourceRegion(src, name string) (string, bool) {
	lines := strings.Split(src, "\n")
	open := "/* doctest:region " + name + " */"
	for i, l := range lines {
		if strings.TrimSpace(l) != open {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "/* doctest:endregion */" {
				return dedent(lines[i+1 : j]), true
			}
		}
		return "", false
	}
	return "", false
}

func dedent(lines []string) string {
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if common > 0 && len(l) >= common {
			out[i] = l[common:]
		} else {
			out[i] = strings.TrimRight(l, " \t")
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func (tc docToolchain) verify(t *testing.T, ex docExample) error {
	switch ex.kind {
	case "run":
		return tc.verifyRun(t, ex)
	case "file":
		if !ex.geblang {
			return nil
		}
		return tc.check(t, ex.arg, ex.code, ex.files)
	case "fragment":
		return tc.verifyFragment(t, ex)
	case "excerpt":
		return tc.verifyExcerpt(ex)
	}
	return nil
}

func (tc docToolchain) verifyChapter(t *testing.T, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	examples, problems := parseDocExamples(string(raw))
	for _, p := range problems {
		t.Errorf("%s:%d: %s", path, p.line, p.msg)
	}
	for _, ex := range examples {
		if ex.kind == "" || ex.kind == "skip" || ex.malformed {
			continue
		}
		t.Run(fmt.Sprintf("%s:%d", filepath.Base(path), ex.line), func(t *testing.T) {
			t.Parallel()
			if err := tc.verify(t, ex); err != nil {
				t.Errorf("%s:%d: %v", path, ex.line, err)
			}
		})
	}
}

// The manual's examples use @allowUnusedImports, which older checkers report as unused imports.
const docsExamplesMinGeblang = "1.47.0"

func TestManualExamples(t *testing.T) {
	bin, framework := scaffoldToolchain(t)
	if out, err := exec.Command(bin, "--version").Output(); err == nil {
		if fields := strings.Fields(string(out)); len(fields) >= 2 && versionLess(fields[1], docsExamplesMinGeblang) {
			t.Skipf("%s is older than %s; manual examples not checked", strings.TrimSpace(string(out)), docsExamplesMinGeblang)
		}
	}
	dir := firstExistingDir("../../gebweb/docs", "../../docs")
	if dir == "" {
		t.Skip("gebweb manual not found")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	tc := docToolchain{bin: bin, framework: framework, contexts: filepath.Join(abs, "doctest"), excerpts: framework}
	files, err := filepath.Glob(filepath.Join(abs, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		tc.verifyChapter(t, path)
	}
}
