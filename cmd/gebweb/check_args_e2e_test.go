package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCliCheckArgsExitsBeforeSetup(t *testing.T) {
	bin, framework := scaffoldToolchain(t)
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("geblang.yaml", "name: argsapp\nversion: 0.4.0\ndependencies:\n  gebweb:\n    path: "+framework+"\n")
	write("main.gb", `module main;
import io;
import gebweb.cli as cli;

export func main(list<string> args): int {
    let cfg = cli.checkArgs(args, {"name": "argsapp"});
    io.println("setup ${cfg["httpPort"]}");
    return 0;
}
`)
	for _, lane := range [][]string{{"run"}, {"run", "--disable-vm"}} {
		t.Run(strings.Join(lane, " "), func(t *testing.T) { checkArgsLane(t, bin, dir, lane) })
	}
}

func checkArgsLane(t *testing.T, bin, dir string, lane []string) {
	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, append(append(append([]string{}, lane...), "main.gb"), args...)...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("run %v: %v", args, err)
			}
			code = exit.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}

	out, errOut, code := run("--helo")
	if code != 2 || !strings.Contains(errOut, "argsapp: unknown option --helo") || !strings.Contains(errOut, "--port N") || strings.Contains(out, "setup") {
		t.Fatalf("--helo: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, _, code = run("--help")
	if code != 0 || !strings.Contains(out, "argsapp server options") || !strings.Contains(out, "--version") || strings.Contains(out, "setup") {
		t.Fatalf("--help: code=%d stdout=%q", code, out)
	}
	out, _, code = run("--version")
	if code != 0 || !strings.HasPrefix(out, "argsapp 0.4.0\n  geblang ") || strings.Contains(out, "setup") {
		t.Fatalf("--version: code=%d stdout=%q", code, out)
	}
	out, _, code = run("--port", "9001")
	if code != 0 || out != "setup 9001\n" {
		t.Fatalf("valid args: code=%d stdout=%q", code, out)
	}
}
