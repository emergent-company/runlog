//go:build !windows

package main

import (
	"os"
	"os/exec"
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
