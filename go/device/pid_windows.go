package device

import "os"

// The simulator exists only on macOS; this keeps the package building elsewhere.
func pidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return pid > 0 && err == nil
}
