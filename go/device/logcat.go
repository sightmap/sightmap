package device

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// logcatLine matches `logcat -v threadtime`:
// "10-29 15:31:15.495  1234  1250 I MyTag   : message".
var logcatLine = regexp.MustCompile(`^\d\d-\d\d \d\d:\d\d:\d\d\.\d+\s+(\d+)\s+\d+\s+([VDIWEFA])\s+(.*?)\s*: ?(.*)$`)

var logcatLevels = map[string]string{"V": "debug", "D": "debug", "I": "info", "W": "warning", "E": "error", "F": "fault", "A": "fault"}

// parseLogcatLine parses one `logcat -v threadtime` line into the pid that
// logged it and an entry whose subsystem is the log tag. System.out and
// System.err (Java's stdout and stderr) read as stdout and stderr.
func parseLogcatLine(line string) (int, ConsoleEntry, bool) {
	m := logcatLine.FindStringSubmatch(line)
	if m == nil {
		return 0, ConsoleEntry{}, false
	}
	pid, _ := strconv.Atoi(m[1])
	e := ConsoleEntry{Level: logcatLevels[m[2]], Source: "logcat", Subsystem: m[3], Message: m[4]}
	switch m[3] {
	case "System.out":
		e.Level, e.Source = "stdout", "stdout"
	case "System.err":
		e.Level, e.Source = "stderr", "stderr"
	}
	return pid, e, true
}

// SetPID tells an Android collector which process is the app, after each
// launch. logcat carries every process's lines; only the app's are kept.
func (l *LogCollector) SetPID(pid int) { l.pid.Store(int64(pid)) }

// logcat streams the device's log from now on, keeping the app's lines.
func (l *LogCollector) logcat(ctx context.Context) {
	for ctx.Err() == nil {
		cmd := l.ADB.command(ctx, "-s", l.UDID, "logcat", "-v", "threadtime", "-T", "1")
		out, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err == nil {
			l.setStreaming(true)
			sc := bufio.NewScanner(out)
			sc.Buffer(make([]byte, 1<<20), 16<<20)
			for sc.Scan() {
				pid, e, ok := parseLogcatLine(sc.Text())
				if !ok {
					continue
				}
				if want := l.pid.Load(); want > 0 && int64(pid) == want {
					l.Console.Push(e)
				}
			}
			err = cmd.Wait()
		}
		l.setStreaming(false)
		if ctx.Err() != nil {
			return
		}
		if l.Log != nil {
			l.Log(fmt.Sprintf("logcat exited (%v); restarting in 1 s", err))
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}
