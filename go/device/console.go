package device

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ConsoleEntry is one line of the app's console: its os_log / Logger output,
// or a line of its stdout or stderr.
type ConsoleEntry struct {
	Index     int    `json:"index"`
	Time      string `json:"time"`
	Level     string `json:"level"`  // debug info default error fault stdout stderr
	Source    string `json:"source"` // os_log, stdout, stderr
	Subsystem string `json:"subsystem,omitempty"`
	Category  string `json:"category,omitempty"`
	Message   string `json:"message"`
}

var levelByType = map[string]string{"Default": "default", "Info": "info", "Debug": "debug", "Error": "error", "Fault": "fault"}

// parseLogLine parses one `log stream --style ndjson` line, or returns false
// for anything that is not a log event.
func parseLogLine(line string) (ConsoleEntry, bool) {
	if !strings.HasPrefix(line, "{") {
		return ConsoleEntry{}, false
	}
	var j struct {
		EventType    string  `json:"eventType"`
		EventMessage *string `json:"eventMessage"`
		MessageType  string  `json:"messageType"`
		Timestamp    string  `json:"timestamp"`
		Subsystem    string  `json:"subsystem"`
		Category     string  `json:"category"`
	}
	if json.Unmarshal([]byte(line), &j) != nil || j.EventType != "logEvent" || j.EventMessage == nil {
		return ConsoleEntry{}, false
	}
	level, ok := levelByType[j.MessageType]
	if !ok {
		level = strings.ToLower(cmpOr(j.MessageType, "default"))
	}
	return ConsoleEntry{Time: j.Timestamp, Level: level, Source: "os_log", Subsystem: j.Subsystem, Category: j.Category, Message: *j.EventMessage}, true
}

// Console is a bounded buffer of console entries with stable, ever-increasing
// indexes.
type Console struct {
	Capacity int

	mu      sync.Mutex
	entries []ConsoleEntry
	next    int
}

// NewConsole returns a Console holding the most recent capacity entries.
func NewConsole(capacity int) *Console { return &Console{Capacity: capacity} }

// Push appends e, assigning its index (and time, when it has none).
func (c *Console) Push(e ConsoleEntry) ConsoleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.Index = c.next
	c.next++
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	c.entries = append(c.entries, e)
	if len(c.entries) > c.Capacity {
		c.entries = c.entries[len(c.entries)-c.Capacity:]
	}
	return e
}

// Len is how many entries have ever been pushed.
func (c *Console) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.next
}

// ConsoleFilter selects entries for List.
type ConsoleFilter struct {
	Limit int    `json:"limit"` // the most recent N; 0 means all
	Grep  string `json:"grep"`  // case-insensitive substring of the message or subsystem
	Level string `json:"level"` // comma-separated levels
	// All includes Apple frameworks' entries (subsystem com.apple.*), which are
	// hidden by default as noise.
	All bool `json:"all"`
}

// ConsoleList is List's result.
type ConsoleList struct {
	Total   int            `json:"total"`
	Entries []ConsoleEntry `json:"entries"`
}

// List returns the entries f selects, oldest first.
func (c *Console) List(f ConsoleFilter) ConsoleList {
	c.mu.Lock()
	defer c.mu.Unlock()
	levels := map[string]bool{}
	for _, l := range strings.Split(f.Level, ",") {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			levels[l] = true
		}
	}
	needle := strings.ToLower(f.Grep)
	var hits []ConsoleEntry
	for _, e := range c.entries {
		if !f.All && strings.HasPrefix(e.Subsystem, "com.apple.") {
			continue
		}
		if len(levels) > 0 && !levels[e.Level] {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(e.Message), needle) && !strings.Contains(strings.ToLower(e.Subsystem), needle) {
			continue
		}
		hits = append(hits, e)
	}
	out := ConsoleList{Total: len(hits), Entries: hits}
	if f.Limit > 0 && len(hits) > f.Limit {
		out.Entries = hits[len(hits)-f.Limit:]
	}
	if out.Entries == nil {
		out.Entries = []ConsoleEntry{}
	}
	return out
}

// Get returns the entry with index i.
func (c *Console) Get(i int) (ConsoleEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		if e.Index == i {
			return e, true
		}
	}
	return ConsoleEntry{}, false
}

// LogCollector fills a Console from the app's process: `log stream` for its
// os_log / Logger output (minus Apple frameworks' info and default chatter) and
// the files its stdout and stderr are redirected to.
type LogCollector struct {
	UDID, Process  string
	Stdout, Stderr string
	Console        *Console
	Log            func(string)

	mu        sync.Mutex
	cancel    context.CancelFunc
	streaming bool
}

// Start begins collecting until Close.
func (l *LogCollector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	go l.stream(ctx)
	go l.tail(ctx, l.Stdout, "stdout")
	go l.tail(ctx, l.Stderr, "stderr")
}

// Streaming reports whether `log stream` is running.
func (l *LogCollector) Streaming() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.streaming
}

func (l *LogCollector) setStreaming(on bool) {
	l.mu.Lock()
	l.streaming = on
	l.mu.Unlock()
}

func (l *LogCollector) stream(ctx context.Context) {
	predicate := fmt.Sprintf(`process == %q AND (NOT (subsystem BEGINSWITH "com.apple.") OR messageType == error OR messageType == fault)`, l.Process)
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, "xcrun", "simctl", "spawn", l.UDID, "log", "stream", "--style", "ndjson", "--level", "info", "--predicate", predicate)
		out, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err == nil {
			l.setStreaming(true)
			sc := bufio.NewScanner(out)
			sc.Buffer(make([]byte, 1<<20), 16<<20)
			for sc.Scan() {
				if e, ok := parseLogLine(sc.Text()); ok {
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
			l.Log(fmt.Sprintf("log stream exited (%v); restarting in 1 s", err))
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// tail follows a file another process appends to.
func (l *LogCollector) tail(ctx context.Context, path, level string) {
	if path == "" {
		return
	}
	var offset int64
	var partial string
	poll := func() {
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		if info.Size() < offset {
			offset = 0
		}
		if info.Size() == offset {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		buf := make([]byte, info.Size()-offset)
		n, _ := f.ReadAt(buf, offset)
		offset += int64(n)
		lines := strings.Split(partial+string(buf[:n]), "\n")
		partial = lines[len(lines)-1]
		for _, line := range lines[:len(lines)-1] {
			if strings.TrimSpace(line) != "" {
				l.Console.Push(ConsoleEntry{Level: level, Source: level, Message: line})
			}
		}
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			poll()
			return
		case <-ticker.C:
			poll()
		}
	}
}

// Close stops collecting.
func (l *LogCollector) Close() {
	if l.cancel != nil {
		l.cancel()
	}
}
