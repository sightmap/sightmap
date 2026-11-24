package device

import (
	"os"
	"os/exec"
)

// The simulator exists only on macOS; this keeps the package building elsewhere.
func pidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return pid > 0 && err == nil
}

// detachProcess is a no-op on Windows, where a child already outlives its
// parent's console.
func detachProcess(cmd *exec.Cmd) {}
