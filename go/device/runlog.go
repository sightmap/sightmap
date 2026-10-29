package device

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// RunsDir is the run-log directory inside the sightmap directory.
const RunsDir = ".device-runs"

// RunEntry is one line of a run log: an action the session took, when, on what
// element, in which view, and whether it worked. A producer agent drives the app
// while consumers follow its run log, lining up their own data (network
// captures, analytics, logs) against the steps by timestamp.
type RunEntry struct {
	TS     string         `json:"ts"`
	Run    string         `json:"run"`
	Seq    int            `json:"seq"`
	Action string         `json:"action"`
	Args   map[string]any `json:"args,omitempty"`
	// Kind and Text are a note's: an annotation another tool records in the
	// run with `run note`, e.g. kind "session-start".
	Kind   string     `json:"kind,omitempty"`
	Text   string     `json:"text,omitempty"`
	Target *RunTarget `json:"target,omitempty"`
	View   string     `json:"view,omitempty"`
	OK     bool       `json:"ok"`
	Error  *Error     `json:"error,omitempty"`
}

// RunTarget is the element an action acted on.
type RunTarget struct {
	ID         string `json:"id"`
	Component  string `json:"component,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Text       string `json:"text,omitempty"`
	Role       string `json:"role"`
}

// RunLog appends one JSON line per step to <sightmapDir>/.device-runs/<run>.jsonl.
type RunLog struct {
	dir string

	mu    sync.Mutex
	run   string
	label string
	seq   int
}

// RunInfo describes the current run.
type RunInfo struct {
	Run   string `json:"run"`
	Path  string `json:"path"`
	Label string `json:"label,omitempty"`
	Seq   int    `json:"seq"`
}

// NewRunLog returns a run log under sightmapDir; Begin starts its first run.
func NewRunLog(sightmapDir string) *RunLog {
	return &RunLog{dir: filepath.Join(sightmapDir, RunsDir)}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func newRunID(label string, now time.Time) string {
	var b [2]byte
	rand.Read(b[:])
	id := now.UTC().Format("20060102T150405Z") + fmt.Sprintf("-%04x", b)
	if slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(label), "-"), "-"); slug != "" {
		id += "-" + truncateRunes(slug, 40)
	}
	return id
}

// Begin starts a new run.
func (r *RunLog) Begin(label string) (RunInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return RunInfo{}, err
	}
	r.run, r.label, r.seq = newRunID(label, time.Now()), label, 0
	return r.info(), nil
}

// Current returns the current run.
func (r *RunLog) Current() RunInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.info()
}

func (r *RunLog) info() RunInfo {
	if r.run == "" {
		return RunInfo{}
	}
	return RunInfo{Run: r.run, Path: filepath.Join(r.dir, r.run+".jsonl"), Label: r.label, Seq: r.seq}
}

// Append stamps e with the time, run and next sequence number and writes it.
func (r *RunLog) Append(e RunEntry) (RunEntry, error) {
	if r.Current().Run == "" {
		if _, err := r.Begin(""); err != nil {
			return e, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.TS, e.Run, e.Seq = time.Now().UTC().Format(time.RFC3339Nano), r.run, r.seq
	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	f, err := os.OpenFile(r.info().Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return e, err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return e, err
}

// LatestRun returns the most recently modified run log under sightmapDir, or "".
func LatestRun(sightmapDir string) string {
	paths, _ := filepath.Glob(filepath.Join(sightmapDir, RunsDir, "*.jsonl"))
	sort.Slice(paths, func(i, j int) bool {
		a, _ := os.Stat(paths[i])
		b, _ := os.Stat(paths[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// ReadRun returns the entries of the run log at path whose seq is after since.
// A partly written last line is skipped, to be read once it is complete.
func ReadRun(path string, since int) ([]RunEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []RunEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e RunEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Seq > since {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}
