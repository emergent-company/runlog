//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
)

// setSysProcAttr is a no-op on Windows (Setsid is not supported).
func setSysProcAttr(cmd *exec.Cmd) {}

// sigterm sends a kill signal to the process on Windows.
// Windows does not support SIGTERM, so we use Process.Kill().
func sigterm(pid int) error {
	proc, err := findProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("kill process %d: %w", pid, err)
	}
	return nil
}

// processAlive returns true if the process with the given PID is still running.
// On Windows, os.FindProcess always succeeds; we attempt OpenProcess instead.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess never errors even for dead PIDs.
	// A zero-wait tells us if it exited.
	state, err := proc.Wait()
	if err != nil {
		// If Wait errors the process may still be running (not our child).
		return true
	}
	return !state.Exited()
}

// trapSigterm registers a handler that calls fn when the process is interrupted.
// On Windows SIGTERM is not standard; we use os.Interrupt instead.
func trapSigterm(fn func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		fn()
	}()
}
