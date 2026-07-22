//go:build windows

package main

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// Windows: no equivalent of Setpgid needed; Chrome detaches naturally.
}

// Process-group / profile reaping is a no-op on Windows; stop falls back to the
// single recorded PID. (The orphan-reaping QoL fix targets the macOS/Unix flow.)
func terminateGroup(pgid int) error               { return nil }
func killGroup(pgid int) error                    { return nil }
func reapByProfile(profilePath string) bool       { return false }
func profileProcessAlive(profilePath string) bool { return false }
