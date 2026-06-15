package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestRelayChildSignalsForwardsTerm verifies an injected termination signal is
// forwarded to the child (a long sleep dies well before the grace timeout).
func TestRelayChildSignalsForwardsTerm(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGTERM

	start := time.Now()
	relayChildSignals(cmd, sigs, done, 5*time.Second)
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("relay hit the grace timeout (%v); signal was not forwarded", elapsed)
	}
	// ProcessState is set once Wait returns; a signal-terminated child is not
	// "Exited" (that reports normal exit), so just confirm it terminated.
	if cmd.ProcessState == nil {
		t.Fatalf("child did not terminate after forwarded signal")
	}
}

// TestRelayChildSignalsCleanExit verifies a child that exits on its own returns
// its exit code with no signal involved.
func TestRelayChildSignalsCleanExit(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	sigs := make(chan os.Signal, 1) // no signal sent

	if code := relayChildSignals(cmd, sigs, done, 5*time.Second); code != 0 {
		t.Fatalf("clean exit code = %d, want 0", code)
	}
}
