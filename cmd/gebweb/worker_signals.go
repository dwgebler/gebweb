package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// runChildWithGracefulSignals forwards SIGINT/SIGTERM to the child, allowing graceTimeout to drain.
func runChildWithGracefulSignals(cmd *exec.Cmd, graceTimeout time.Duration) int {
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb worker: %v\n", err)
		return 1
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return relayChildSignals(cmd, sigs, done, graceTimeout)
}

// relayChildSignals forwards a termination signal to the child, then hard-kills after graceTimeout.
func relayChildSignals(cmd *exec.Cmd, sigs <-chan os.Signal, done <-chan error, graceTimeout time.Duration) int {
	select {
	case err := <-done:
		return childExitCode(err)
	case <-sigs:
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case err := <-done:
			return childExitCode(err)
		case <-time.After(graceTimeout):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
			return 1
		}
	}
}

func childExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
