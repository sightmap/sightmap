//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A daemon killed during startup must not leave its children (adb logcat, the
// simulator helper) running.
func TestStopUnstartedDaemonStopsChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("sh", "-c", `sleep 300 & echo $! > "$1"; wait`, "sh", pidFile)
	configureDetachedDaemon(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var child int
	for deadline := time.Now().Add(5 * time.Second); child == 0; {
		if b, err := os.ReadFile(pidFile); err == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if child == 0 && time.Now().After(deadline) {
			t.Fatal("child never started")
		}
		time.Sleep(20 * time.Millisecond)
	}

	stopUnstartedDaemon(cmd.Process, done)

	for deadline := time.Now().Add(5 * time.Second); syscall.Kill(child, 0) == nil; {
		if time.Now().After(deadline) {
			syscall.Kill(child, syscall.SIGKILL)
			t.Fatalf("child %d still running after the daemon was stopped", child)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
