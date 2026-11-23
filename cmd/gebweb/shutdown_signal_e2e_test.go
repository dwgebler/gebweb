package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCliShutdownLogsPhasesAndHonoursASecondSignal(t *testing.T) {
	bin, framework := scaffoldToolchain(t)
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("geblang.yaml", "name: stopapp\nversion: 1.0.0\ndependencies:\n  gebweb:\n    path: "+framework+"\n")
	write("main.gb", `module main;
import sys;
import time;
import gebweb;

export func main(list<string> args): int {
    let app = gebweb.app([]);
    int hookMs = (sys.getenv("HOOK_MS") ?? "0") as int;
    gebweb.onShutdown(app, "flush", func(): void { time.sleep(hookMs); });
    gebweb.cli(app, {"name": "stopapp", "port": 0});
    return 0;
}
`)
	built := filepath.Join(dir, "out", "stopapp")
	if out, err := exec.Command(bin, "build", "--entry", "main", "--out", built, dir).CombinedOutput(); err != nil {
		t.Fatalf("geblang build: %v\n%s", err, out)
	}
	lanes := map[string][]string{
		"vm":        {bin, "run", "main.gb"},
		"evaluator": {bin, "run", "--disable-vm", "main.gb"},
		"built":     {built},
	}
	for name, lane := range lanes {
		t.Run(name+" clean", func(t *testing.T) {
			out, code := runUntilSignals(t, bin, dir, lane, "0", 1)
			if code != 0 {
				t.Fatalf("exit %d, output:\n%s", code, out)
			}
			for _, want := range []string{
				"stopapp: received SIGINT, shutting down (timeout 10s)",
				"stopapp: stopped accepting connections; in-flight requests finished (",
				`stopapp: shutdown hook "flush" done (`,
				"stopapp: stopped cleanly in ",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in:\n%s", want, out)
				}
			}
		})
		t.Run(name+" forced", func(t *testing.T) {
			out, code := runUntilSignals(t, bin, dir, lane, "5000", 2)
			if code != 130 || !strings.Contains(out, "stopapp: received SIGINT again, exiting immediately") {
				t.Fatalf("exit %d, output:\n%s", code, out)
			}
		})
	}
}

func runUntilSignals(t *testing.T, bin, dir string, lane []string, hookMs string, signals int) (string, int) {
	t.Helper()
	cmd := exec.Command(lane[0], lane[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOOK_MS="+hookMs, "GEBWEB_LOG_FORMAT=")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		reader := bufio.NewReader(stdout)
		signalled := false
		for {
			line, err := reader.ReadString('\n')
			out.WriteString(line)
			if !signalled && strings.Contains(line, "serving http://") {
				signalled = true
				close(ready)
			}
			if err != nil {
				if err != io.EOF {
					t.Errorf("read: %v", err)
				}
				return
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("server never started:\n%s\n%s", out.String(), stderr.String())
	}
	for i := 0; i < signals; i++ {
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	waitErr := cmd.Wait()
	<-done
	code := 0
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		code = exit.ExitCode()
	} else if waitErr != nil {
		t.Fatal(waitErr)
	}
	return out.String() + stderr.String(), code
}
