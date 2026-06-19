//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// setSysProcAttr configures the command to run in a new session (Unix only).
// This ensures the child process survives if the TUI is closed.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// sigterm sends SIGTERM to the given process (Unix only).
func sigterm(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// processAlive returns true if the process with the given PID is still running.
// Uses signal 0 which does not actually send a signal but checks for existence.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// trapSigterm registers a handler that calls fn when SIGTERM or SIGINT
// (Ctrl+C) is received.  SIGINT is included so foreground-mode daemons
// shut down cleanly instead of getting "signal: killed".
func trapSigterm(fn func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		fn()
	}()
}
