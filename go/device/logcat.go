package device

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

// startProc matches ActivityManager's line for each process it starts, which
// it logs before the process logs anything:
// "Start proc 17738:com.example.app/u0a190 for top-activity {…}".
var startProc = regexp.MustCompile(`^Start proc (\d+):([^/\s]+)/`)

// heldLines is how many recent lines of processes not known to be the app's
// are kept, for a pid learned (from pidof) after its first lines went by.
const heldLines = 2000

type heldLine struct {
	pid int
	e   ConsoleEntry
}

// appLines picks the app's lines out of logcat, which carries every process's.
// It learns the app's pids from ActivityManager's "Start proc" lines, so a
// launch, a restart or a start by hand is followed from its first line.
type appLines struct {
	pkg  string
	push func(ConsoleEntry)

	mu   sync.Mutex
	pids map[int]bool
	held []heldLine
}

func newAppLines(pkg string, push func(ConsoleEntry)) *appLines {
	return &appLines{pkg: pkg, push: push, pids: map[int]bool{}}
}

// watch adds pid as the app's, keeping the held lines it already logged.
func (a *appLines) watch(pid int) {
	if pid <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.watchLocked(pid)
}

func (a *appLines) watchLocked(pid int) {
	if a.pids[pid] {
		return
	}
	a.pids[pid] = true
	kept := a.held[:0]
	for _, h := range a.held {
		if h.pid == pid {
			a.push(h.e)
		} else {
			kept = append(kept, h)
		}
	}
	a.held = kept
}

func (a *appLines) line(pid int, e ConsoleEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if e.Subsystem == "ActivityManager" {
		if m := startProc.FindStringSubmatch(e.Message); m != nil {
			started, _ := strconv.Atoi(m[1])
			// A secondary process is named pkg:suffix.
			if name := m[2]; name == a.pkg || strings.HasPrefix(name, a.pkg+":") {
				a.watchLocked(started)
			} else {
				delete(a.pids, started)
			}
		}
	}
	if a.pids[pid] {
		a.push(e)
		return
	}
	if len(a.held) == heldLines {
		a.held = append(a.held[:0], a.held[heldLines/2:]...)
	}
	a.held = append(a.held, heldLine{pid, e})
}

func (l *LogCollector) appLines() *appLines {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.apps == nil {
		l.apps = newAppLines(l.Process, func(e ConsoleEntry) { l.Console.Push(e) })
	}
	return l.apps
}

// SetPID tells an Android collector which process is the app, after each
// launch, in case logcat didn't show it starting.
func (l *LogCollector) SetPID(pid int) { l.appLines().watch(pid) }

// logcat streams the device's log from now on, keeping the app's lines. While
// the device is away it retries ever less often, logging the first failure,
// every tenth, and the recovery.
func (l *LogCollector) logcat(ctx context.Context) {
	apps := l.appLines()
	sleep := l.sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) {
			select {
			case <-ctx.Done():
			case <-time.After(d):
			}
		}
	}
	failures := 0
	for ctx.Err() == nil {
		cmd := l.ADB.command(ctx, "-s", l.UDID, "logcat", "-v", "threadtime", "-T", "1")
		out, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err == nil {
			l.setStreaming(true)
			// An app already running (attached to, or started while logcat was
			// down) has no "Start proc" line ahead.
			apps.watch(AndroidSim{ADB: l.ADB}.pid(ctx, l.UDID, l.Process))
			sc := bufio.NewScanner(out)
			sc.Buffer(make([]byte, 1<<20), 16<<20)
			for sc.Scan() {
				if failures > 0 {
					if l.Log != nil {
						l.Log("logcat is streaming again")
					}
					failures = 0
				}
				if pid, e, ok := parseLogcatLine(sc.Text()); ok {
					apps.line(pid, e)
				}
			}
			err = cmd.Wait()
		}
		l.setStreaming(false)
		if ctx.Err() != nil {
			return
		}
		failures++
		delay := min(time.Second<<min(failures-1, 5), 30*time.Second)
		if l.Log != nil && (failures == 1 || failures%10 == 0) {
			l.Log(fmt.Sprintf("logcat exited (%v, %d in a row); restarting in %s", err, failures, delay))
		}
		sleep(ctx, delay)
	}
}
