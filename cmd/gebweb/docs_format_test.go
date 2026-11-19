package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var gbFence = regexp.MustCompile("(?ms)^([ \t]*)```(?:gb|geblang)[ \t]*\n(.*?)^[ \t]*```[ \t]*$")

// Code blocks that parse must already be in geblang fmt form; fragments that do not parse are skipped.
func TestManualCodeBlocksAreFormatted(t *testing.T) {
	bin := geblangForDocs(t)
	dir := firstExistingDir("../../gebweb/docs", "../../docs")
	if dir == "" {
		t.Skip("gebweb manual not found")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		for _, m := range gbFence.FindAllStringSubmatchIndex(src, -1) {
			indent := src[m[2]:m[3]]
			code := src[m[4]:m[5]]
			if indent != "" {
				lines := strings.Split(code, "\n")
				for i, l := range lines {
					lines[i] = strings.TrimPrefix(l, indent)
				}
				code = strings.Join(lines, "\n")
			}
			cmd := exec.Command(bin, "fmt", "--stdin")
			cmd.Stdin = strings.NewReader(code)
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			if got := strings.TrimRight(string(out), "\n") + "\n"; got != code {
				line := strings.Count(src[:m[0]], "\n") + 1
				t.Errorf("%s:%d: code block is not geblang fmt formatted; want:\n%s", path, line, got)
			}
		}
	}
}

// The manual is formatted with this geblang release's fmt rules; older formatters disagree.
const docsFmtMinGeblang = "1.43.0"

// geblangForDocs prefers the repo build over PATH so the check uses the formatter under test.
func geblangForDocs(t *testing.T) string {
	bin := ""
	if p, err := filepath.Abs("../../geblang"); err == nil {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			bin = p
		}
	}
	if bin == "" {
		p, err := exec.LookPath("geblang")
		if err != nil {
			t.Skip("geblang binary not available")
		}
		bin = p
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Skipf("geblang --version failed: %v", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || versionLess(fields[1], docsFmtMinGeblang) {
		t.Skipf("%s is older than %s; manual formatting not checked", strings.TrimSpace(string(out)), docsFmtMinGeblang)
	}
	return bin
}

func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func firstExistingDir(candidates ...string) string {
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}
