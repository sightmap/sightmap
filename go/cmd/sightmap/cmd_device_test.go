package main

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/device"
	"github.com/sightmap/sightmap/go/sightmap"
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

func TestDeviceNetworkFlags(t *testing.T) {
	req, err := deviceRequest("network", []string{"list", "--grep", "orders", "--source", "har", "--since", "4", "--follow", "--unredacted"}, newDeviceFlags("network"))
	if err != nil {
		t.Fatal(err)
	}
	f := req.args.(device.NetworkFilter)
	if req.cmd != "networkList" || !req.follow || f.Grep != "orders" || f.Source != "har" || f.Since == nil || *f.Since != 4 || !f.Unredacted {
		t.Fatalf("req = %+v, filter %+v", req, f)
	}
	req, err = deviceRequest("network", []string{"get", "7", "--response-file", "rsp.json"}, newDeviceFlags("network"))
	if err != nil {
		t.Fatal(err)
	}
	if a := req.args.(device.ValueArgs); req.cmd != "networkGet" || a.Index != 7 || a.Unredacted || !strings.HasSuffix(req.rspFile, "/rsp.json") {
		t.Fatalf("req = %+v", req)
	}
	if _, err := deviceRequest("network", []string{"get", "x"}, newDeviceFlags("network")); err == nil {
		t.Error("a non-numeric index was accepted")
	}
}

func TestNetworkLines(t *testing.T) {
	l := device.NetworkList{Total: 2, Last: 5, Capacity: 1000, Entries: []device.NetworkEntry{
		{Index: 4, Source: "har", Method: "POST", URL: "https://api.test/orders", Status: 201, DurationMs: 12, Matches: []string{"CreateOrder"}, Props: []sightmap.PropertyValue{{Name: "order", Value: "2"}}},
		{Index: 5, Source: "summary", Status: 500, Error: "failure"},
	}}
	out := strings.Join(networkLines(l, false), "\n")
	for _, want := range []string{"#4 [CreateOrder]", "POST https://api.test/orders → 201 (12 ms, har) {order=2}", "#5 [--]", "(host not logged) → 500 failure (summary)"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
}

func TestNetworkCheck(t *testing.T) {
	c := networkCheck(device.NetStatus{Mode: "auto", Sources: []device.NetSourceStatus{
		{Name: "har", State: "unavailable", Detail: "SimSample lacks the get-task-allow entitlement"},
		{Name: "summary", State: "active", Detail: "host only"},
	}})
	if !c.OK || !strings.Contains(c.Detail, "active: summary (host only)") || !strings.Contains(c.Detail, "har unavailable: SimSample lacks") {
		t.Errorf("check = %+v", c)
	}
	c = networkCheck(device.NetStatus{Mode: "har", Sources: []device.NetSourceStatus{{Name: "har", State: "unavailable", Detail: "x"}}})
	if c.OK || c.Fix == "" {
		t.Errorf("check = %+v", c)
	}
}

func TestDevicePlatform(t *testing.T) {
	for _, c := range []struct{ flag, app, pkg, want string }{
		{"", "/x/app-debug.apk", "", device.PlatformAndroid},
		{"", "", "com.example", device.PlatformAndroid},
		{"Android", "", "", device.PlatformAndroid},
		{"ios", "/x/My.app", "", device.PlatformIOS},
	} {
		got, err := devicePlatform(c.flag, c.app, c.pkg)
		if err != nil || got != c.want {
			t.Errorf("devicePlatform(%q, %q, %q) = %q, %v; want %q", c.flag, c.app, c.pkg, got, err, c.want)
		}
	}
	if _, err := devicePlatform("ios", "/x/app.apk", ""); device.Code(err) != "usage" {
		t.Errorf("--platform ios with an .apk = %v", err)
	}
	if _, err := devicePlatform("windows", "", ""); device.Code(err) != "usage" {
		t.Errorf("--platform windows = %v", err)
	}
}
