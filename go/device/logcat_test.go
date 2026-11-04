package device

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeADBScript writes an adb stand-in that runs body for `logcat` and fails
// everything else (so pidof finds nothing).
func fakeADBScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script for adb")
	}
	path := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\ncase \"$*\" in\n*logcat*)\n" + body + "\n;;\nesac\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAndroidConsoleFollowsTheAppsProcessesFromTheirFirstLine(t *testing.T) {
	adb := fakeADBScript(t, `cat <<'EOF'
--------- beginning of main
11-02 16:22:21.100   100   100 D Other   : before the app starts
11-02 16:22:21.168   569   627 I ActivityManager: Start proc 4321:com.example.app/u0a190 for top-activity {com.example.app/com.example.app.MainActivity}
11-02 16:22:21.192  4321  4321 I Startup : Application.onCreate
11-02 16:22:21.193   100   100 D Other   : another process
11-02 16:22:21.194  4321  4321 I System.out: hello from stdout
11-02 16:22:21.300   569   627 I ActivityManager: Start proc 4400:com.example.app:sync/u0a190 for service {com.example.app/com.example.app.SyncService}
11-02 16:22:21.301  4400  4400 I Sync    : secondary process
11-02 16:22:22.000   569   627 I ActivityManager: Start proc 4321:com.example.other/u0a200 for top-activity {com.example.other/.Main}
11-02 16:22:22.001  4321  4321 I Other   : a reused pid
11-02 16:22:23.000  5000  5000 I Late    : logged before its pid was known
EOF
exec sleep 60`)
	l := &LogCollector{UDID: "emulator-5554", Process: "com.example.app", Console: NewConsole(100), Platform: PlatformAndroid, ADB: ADB{Path: adb}}
	l.Start()
	defer l.Close()
	waitFor := func(n int) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); l.Console.Len() < n && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
	}
	// No launch told the collector the app's pid (as when launching fails and
	// the app is started by hand).
	waitFor(3)
	l.SetPID(5000)
	waitFor(4)
	time.Sleep(100 * time.Millisecond)
	var got []string
	for _, e := range l.Console.List(ConsoleFilter{}).Entries {
		got = append(got, e.Subsystem+": "+e.Message)
	}
	want := []string{
		"Startup: Application.onCreate",
		"System.out: hello from stdout",
		"Sync: secondary process",
		"Late: logged before its pid was known",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("console:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLogcatRestartsBackOffAndLogSparingly(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	adb := fakeADBScript(t, `n=$(cat `+count+` 2>/dev/null || echo 0); n=$((n+1)); echo $n > `+count+`
[ $n -le 12 ] && exit 1
echo '11-02 16:22:21.100   100   100 D Other   : a line'
exit 0`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs []string
	var delays []time.Duration
	l := &LogCollector{UDID: "emulator-5554", Process: "com.example.app", Console: NewConsole(100), Platform: PlatformAndroid, ADB: ADB{Path: adb},
		Log: func(s string) { logs = append(logs, s) },
		sleep: func(_ context.Context, d time.Duration) {
			delays = append(delays, d)
			if len(delays) == 13 {
				cancel()
			}
		}}
	l.logcat(ctx)
	s := time.Second
	want := []time.Duration{s, 2 * s, 4 * s, 8 * s, 16 * s, 30 * s, 30 * s, 30 * s, 30 * s, 30 * s, 30 * s, 30 * s, s}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
	if len(logs) != 4 || !strings.Contains(logs[0], "1 in a row") || !strings.Contains(logs[1], "10 in a row") ||
		logs[2] != "logcat is streaming again" || !strings.Contains(logs[3], "1 in a row") {
		t.Errorf("logs = %q", logs)
	}
}
