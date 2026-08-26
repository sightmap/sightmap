//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// Windows: no equivalent of Setpgid needed; Chrome detaches naturally.
}

// configureDetachedDaemon prepares cmd to run as a background daemon dissociated
// from the launching console/process group, so `browser start --detach` survives
// the shell that launched it. DETACHED_PROCESS (0x8) | CREATE_NEW_PROCESS_GROUP
// (0x200).
func configureDetachedDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
}

// Process-group / profile reaping is a no-op on Windows; stop falls back to the
// single recorded PID. (The orphan-reaping QoL fix targets the macOS/Unix flow.)
func terminateGroup(pgid int) error               { return nil }
func killGroup(pgid int) error                    { return nil }
func reapByProfile(profilePath string) bool       { return false }
func profileProcessAlive(profilePath string) bool { return false }
