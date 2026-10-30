package main

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/device"
)

func TestDeviceConsoleListFlags(t *testing.T) {
	req, err := deviceRequest("console", []string{"list", "--grep", "session", "-i", "--since", "12", "--follow"}, newDeviceFlags("console"))
	if err != nil {
		t.Fatal(err)
	}
	f := req.args.(device.ConsoleFilter)
	if req.cmd != "consoleList" || !req.follow || f.Grep != "session" || f.Since == nil || *f.Since != 12 {
		t.Fatalf("req = %+v, filter %+v", req, f)
	}
	req, _ = deviceRequest("console", []string{"list"}, newDeviceFlags("console"))
	if req.args.(device.ConsoleFilter).Since != nil {
		t.Error("--since is unset by default")
	}
}

func TestConsoleLinesReportDroppedAndPaging(t *testing.T) {
	l := device.ConsoleList{Total: 3, Last: 9, Dropped: 4, Capacity: 5,
		Entries: []device.ConsoleEntry{{Index: 9, Level: "stdout", Message: "hi"}}}
	out := strings.Join(consoleLines(l, false), "\n")
	for _, want := range []string{"(4 earlier entries dropped; the console keeps the latest 5)", "#9 stdout", "--since 9"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}

func TestFormatRunNote(t *testing.T) {
	got := formatRunEntry(device.RunEntry{Seq: 3, TS: "T", Action: "note", Kind: "session-start", Text: "recording"})
	if got != "3 T note [session-start] recording" {
		t.Errorf("got %q", got)
	}
}
