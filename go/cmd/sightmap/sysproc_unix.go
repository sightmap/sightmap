//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// setSysProcAttr sets Chrome to run in its own process group so it isn't
// killed when the browser CLI process exits or receives a terminal signal.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup sends SIGTERM to an entire process group (negative pid), so
// Chrome's helper/renderer children are reaped too — not just the leader.
func terminateGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGTERM)
}

// killGroup sends SIGKILL to an entire process group.
func killGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}

// reapByProfile force-terminates any process whose command line contains
// profilePath (a Chrome --user-data-dir). This catches orphans the recorded
// session id can't address — including the no-session-file case and macOS PID
// reuse where the recorded id missed the real browser. Best-effort via pkill.
// Returns true if it terminated at least one process.
func reapByProfile(profilePath string) bool {
	if profilePath == "" {
		return false
	}
	term := exec.Command("pkill", "-TERM", "-f", profilePath).Run() == nil
	if term {
		time.Sleep(700 * time.Millisecond)
	}
	kill := exec.Command("pkill", "-KILL", "-f", profilePath).Run() == nil
	return term || kill
}

// profileProcessAlive reports whether any process's command line contains
// profilePath. Best-effort via pgrep (exit 0 = match found).
func profileProcessAlive(profilePath string) bool {
	if profilePath == "" {
		return false
	}
	return exec.Command("pgrep", "-f", profilePath).Run() == nil
}
