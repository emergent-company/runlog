package main

import (
	"fmt"
	"os"
)

// findProcess wraps os.FindProcess with a cleaner error message.
func findProcess(pid int) (*os.Process, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("find process %d: %w", pid, err)
	}
	return proc, nil
}
