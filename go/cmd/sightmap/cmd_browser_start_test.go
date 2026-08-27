package main

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSandboxHint(t *testing.T) {
	// A sandbox-signature stderr yields a --no-sandbox hint...
	if h := sandboxHint("Failed to move to new namespace: No usable sandbox!", false); !strings.Contains(h, "--no-sandbox") {
		t.Errorf("expected a --no-sandbox hint, got %q", h)
	}
	// ...unless --no-sandbox is already set (then the failure is something else).
	if h := sandboxHint("No usable sandbox!", true); h != "" {
		t.Errorf("expected no hint when --no-sandbox already present, got %q", h)
	}
	// Unrelated failures get no sandbox hint.
	if h := sandboxHint("Missing X server or $DISPLAY", false); h != "" {
		t.Errorf("expected no hint for a non-sandbox failure, got %q", h)
	}
}

func TestShouldAutoHeadless(t *testing.T) {
	if runtime.GOOS != "linux" {
		// Off Linux there is always a usable display; never auto-headless.
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		if shouldAutoHeadless() {
			t.Errorf("shouldAutoHeadless must be false off Linux")
		}
		return
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if !shouldAutoHeadless() {
		t.Errorf("expected auto-headless on Linux with no display")
	}
	t.Setenv("DISPLAY", ":0")
	if shouldAutoHeadless() {
		t.Errorf("expected no auto-headless when DISPLAY is set")
	}
}

func TestStripStartFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		strip []string
		want  []string
	}{
		{"bool flag alone", []string{"--detach"}, []string{"detach", "log-file"}, nil},
		{"bool =true form", []string{"--detach=true"}, []string{"detach", "log-file"}, nil},
		{"single dash", []string{"-detach"}, []string{"detach", "log-file"}, nil},
		{
			"keeps other flags",
			[]string{"--headless", "--detach", "--port", "9"},
			[]string{"detach", "log-file"},
			[]string{"--headless", "--port", "9"},
		},
		{
			"value flag with separate value token",
			[]string{"--log-file", "/tmp/x", "--url", "http://y"},
			[]string{"detach", "log-file"},
			[]string{"--url", "http://y"},
		},
		{
			"value flag =form",
			[]string{"--log-file=/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"both stripped",
			[]string{"--detach", "--log-file", "/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"does not eat the token after a bool flag",
			[]string{"--detach", "--port", "7890"},
			[]string{"detach", "log-file"},
			[]string{"--port", "7890"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStartFlags(tc.args, tc.strip...)
			if !slices.Equal(got, tc.want) {
				t.Errorf("stripStartFlags(%v, %v) = %v, want %v", tc.args, tc.strip, got, tc.want)
			}
		})
	}
}
