// Package evals hosts opt-in browser measurements, not model effectiveness claims.
package evals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

type task struct {
	ID, Prompt, Assert string
	Actions            []string
}

var tasks = []task{
	{"save-contact", "Save a contact named Ada.", `document.querySelector('#saved').textContent === 'Saved: Ada'`, []string{`document.querySelector('#name').value='Ada'; document.querySelector('#name').dispatchEvent(new Event('input',{bubbles:true})); true`, `document.querySelector('#contact button').click(); true`}},
	{"filter-select", "Filter the directory to Grace and select her.", `document.querySelector('#selected').textContent === 'Selected: Grace' && document.querySelector('[data-key="Ada"]').hidden && !document.querySelector('[data-key="Grace"]').hidden`, []string{`document.querySelector('#filter').value='Grace'; document.querySelector('#filter').dispatchEvent(new Event('input',{bubbles:true})); true`, `document.querySelector('[data-person="Grace"]').click(); true`}},
	{"open-report", "Open the report and find the quarter total.", `location.hash === '#report' && !document.querySelector('#report').hidden && document.querySelector('#report').textContent === 'Quarter total: 42'`, []string{`document.querySelector('a').click(); new Promise(resolve => setTimeout(() => resolve(true), 20))`}},
}

type cell struct {
	Task             string   `json:"task"`
	Mode             string   `json:"mode"`
	Repetition       int      `json:"repetition"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason,omitempty"`
	Success          *bool    `json:"scripted_assertion_success"`
	ModelTokens      *int     `json:"model_tokens"`
	Actions          *int     `json:"scripted_actions"`
	ObservationBytes *int     `json:"observation_bytes"`
	WallMS           *float64 `json:"wall_ms"`
	CaptureMS        *float64 `json:"capture_ms"`
	Artifact         string   `json:"artifact,omitempty"`
}
type event struct {
	Kind    string  `json:"kind"`
	Content string  `json:"content"`
	WallMS  float64 `json:"wall_ms"`
}
type result struct {
	RunComplete   bool              `json:"run_complete"`
	RunnerSHA     string            `json:"runner_sha256"`
	Dirty         bool              `json:"source_dirty"`
	SchemaVersion int               `json:"schema_version"`
	ExecutionKind string            `json:"execution_kind"`
	Revision      string            `json:"source_revision"`
	SourceDiffSHA string            `json:"source_diff_sha256"`
	Generated     string            `json:"generated_at"`
	Go            string            `json:"go_version"`
	Platform      string            `json:"platform"`
	Browser       json.RawMessage   `json:"browser"`
	FixtureHashes map[string]string `json:"fixture_sha256"`
	Tasks         []task            `json:"tasks"`
	Cells         []cell            `json:"cells"`
}

// rawCDP preserves the complete CDP result, including AX hierarchy and fields
// omitted by extract.A11YNode. It uses an independent connection to our owned tab.
type rawCDP struct {
	ws   *websocket.Conn
	next int
}

func (c *rawCDP) call(method string) (json.RawMessage, error) {
	c.next++
	if err := c.ws.WriteJSON(map[string]any{"id": c.next, "method": method}); err != nil {
		return nil, err
	}
	c.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := c.ws.ReadJSON(&msg); err != nil {
			return nil, err
		}
		if msg.ID == c.next {
			if len(msg.Error) > 0 {
				return nil, fmt.Errorf("%s: %s", method, msg.Error)
			}
			return msg.Result, nil
		}
	}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, e := json.MarshalIndent(v, "", "  ")
	must(t, e)
	must(t, os.WriteFile(path, append(b, '\n'), 0644))
}
func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func TestMatrix(t *testing.T) {
	chrome := os.Getenv("SIGHTMAP_EVAL_CHROME")
	if chrome == "" {
		t.Skip("set SIGHTMAP_EVAL_CHROME to an explicit Chrome executable to run isolated browser measurements")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture, err := filepath.Abs("../../evals/fixtures/office")
	must(t, err)
	server := httptest.NewServer(http.FileServer(http.Dir(fixture)))
	defer server.Close()
	profile := t.TempDir()
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--remote-debugging-port=0", "--user-data-dir="+profile, "about:blank")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	must(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var addr string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		b, e := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		if e == nil {
			addr = "127.0.0.1:" + strings.Split(string(b), "\n")[0]
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr == "" {
		t.Fatalf("Chrome did not expose its isolated debugging port")
	}
	conn, err := browser.Connect(addr, "")
	must(t, err)
	defer conn.Close()
	must(t, browser.NavigateAndWait(ctx, conn, server.URL+"/"))
	var targets []struct {
		Type string `json:"type"`
		WS   string `json:"webSocketDebuggerUrl"`
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/list", nil)
	must(t, err)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	must(t, err)
	must(t, json.NewDecoder(resp.Body).Decode(&targets))
	resp.Body.Close()
	var wsURL string
	for _, target := range targets {
		if target.Type == "page" {
			wsURL = target.WS
		}
	}
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	must(t, err)
	defer ws.Close()
	raw := &rawCDP{ws: ws}
	// Optional authoring pass: verify every proposed selector before creating YAML.
	if binary := os.Getenv("SIGHTMAP_EVAL_SELECTOR_BINARY"); binary != "" {
		for _, name := range []string{"ContactName", "SaveContact", "ContactResult", "DirectoryFilter", "SelectPerson", "SelectionResult", "ReportLink", "ReportValue"} {
			out, e := exec.CommandContext(ctx, binary, "sel-probe", "--addr", addr, "--", `[data-component="`+name+`"]`).CombinedOutput()
			must(t, e)
			t.Logf("%s: %s", name, out)
		}
		if os.Getenv("SIGHTMAP_EVAL_AUTHOR_ONLY") == "1" {
			return
		}
	}
	corpus, err := sightmap.Load(filepath.Join(fixture, ".sightmap"))
	must(t, err)
	out := os.Getenv("SIGHTMAP_EVAL_OUT")
	if out == "" {
		out = t.TempDir()
	}
	must(t, os.MkdirAll(out, 0755))
	git := func(args ...string) []byte {
		b, e := exec.Command("git", args...).Output()
		must(t, e)
		return bytes.TrimSpace(b)
	}
	r := result{SchemaVersion: 1, ExecutionKind: "scripted", Revision: string(git("rev-parse", "HEAD")), SourceDiffSHA: hash(git("diff", "HEAD", "--", ":/")), Generated: time.Now().UTC().Format(time.RFC3339), Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, FixtureHashes: map[string]string{}, Tasks: tasks}
	runnerBytes, err := os.ReadFile("matrix_test.go")
	must(t, err)
	r.RunnerSHA = hash(runnerBytes)
	r.Dirty = len(git("status", "--porcelain", "--untracked-files=normal")) > 0
	defer func() { writeJSON(t, filepath.Join(out, "results.json"), r) }()
	r.Browser, err = raw.call("Browser.getVersion")
	must(t, err)
	for _, path := range []string{"index.html", ".sightmap/app.yaml"} {
		b, e := os.ReadFile(filepath.Join(fixture, path))
		must(t, e)
		r.FixtureHashes[path] = hash(b)
	}
	failed := false
	for repetition := 1; repetition <= 3; repetition++ {
		for _, task := range tasks {
			modes := []string{"raw-ax", "sightmap", "sightkick", "stagehand-facade"}
			if repetition%2 == 0 {
				modes[0], modes[1] = modes[1], modes[0]
			}
			for _, mode := range modes {
				c := cell{Task: task.ID, Mode: mode, Repetition: repetition, Status: "not_run", Reason: "adapter not implemented; no model trial performed"}
				if mode == "sightkick" || mode == "stagehand-facade" {
					r.Cells = append(r.Cells, c)
					continue
				}
				func() {
					events := []event{}
					size, count := 0, 0
					captureMS, elapsed := 0.0, 0.0
					success := false
					c.Status = "failed"
					c.Reason = "trial interrupted"
					c.Success = &success
					c.Actions = &count
					c.ObservationBytes = &size
					c.WallMS = &elapsed
					c.CaptureMS = &captureMS
					c.Artifact = fmt.Sprintf("%s-%s-%d.json", task.ID, mode, repetition)
					defer func() {
						if !success {
							failed = true
						}
						writeJSON(t, filepath.Join(out, c.Artifact), events)
						r.Cells = append(r.Cells, c)
						writeJSON(t, filepath.Join(out, "results.json"), r)
					}()
					// Full navigation resets DOM and hash before every pair.
					if e := browser.NavigateAndWait(ctx, conn, server.URL+"/"); e != nil {
						c.Reason = "reset: " + e.Error()
						return
					}
					initial, e := browser.EvalJSON(ctx, conn, task.Assert)
					if e != nil {
						c.Reason = "precondition: " + e.Error()
						return
					}
					events = append(events, event{"precondition", task.Assert + " => " + string(initial), 0})
					if string(initial) != "false" {
						c.Reason = "oracle already succeeds before actions"
						return
					}
					start := time.Now()
					c.Status = "completed"
					c.Reason = ""
					for _, action := range task.Actions {
						before := time.Now()
						var observation []byte
						if mode == "raw-ax" {
							observation, e = raw.call("Accessibility.getFullAXTree")
						} else {
							var observed *observe.Result
							observed, e = observe.Page(ctx, conn, corpus, observe.Options{VisibleOnly: true})
							if e == nil {
								if observed.Coverage.Total == 0 || observed.Coverage.T3 != 0 {
									e = fmt.Errorf("invalid corpus coverage: total=%d orphaned=%d", observed.Coverage.Total, observed.Coverage.T3)
								} else {
									var buf bytes.Buffer
									observe.Format(&buf, observed, observe.FormatOpts{})
									observation = buf.Bytes()
								}
							}
						}
						if e != nil {
							c.Status = "failed"
							c.Reason = e.Error()
							break
						}
						captureMS += float64(time.Since(before).Microseconds()) / 1000
						size += len(observation)
						events = append(events, event{"observation", string(observation), float64(time.Since(before).Microseconds()) / 1000})
						before = time.Now()
						_, e = browser.EvalJSON(ctx, conn, action)
						count++
						events = append(events, event{"scripted_action", action, float64(time.Since(before).Microseconds()) / 1000})
						if e != nil {
							c.Status = "failed"
							c.Reason = e.Error()
							break
						}
					}
					elapsed = float64(time.Since(start).Microseconds()) / 1000
					final, e := browser.EvalJSON(ctx, conn, task.Assert)
					if e != nil {
						c.Status = "failed"
						c.Reason = e.Error()
					} else {
						success = string(final) == "true" && c.Status == "completed"
					}
					events = append(events, event{"oracle", task.Assert + " => " + string(final), 0})
					if !success {
						failed = true
						c.Status = "failed"
						if c.Reason == "" {
							c.Reason = "final-state predicate failed"
						}
					}
				}()
			}
		}
	}
	r.RunComplete = true
	must(t, validate(r.Cells))
	writeJSON(t, filepath.Join(out, "results.json"), r)
	var report strings.Builder
	report.WriteString("# Scripted browser smoke measurements\n\nThese are observation and fixture checks, not agent effectiveness results. Model tokens are null throughout. Wall time includes observations and scripted actions, excluding setup/reset and final assertions.\n\n| Task | Mode | Trial | Assertion | Actions | Observation bytes | Capture ms | Wall ms |\n|---|---|---:|---|---:|---:|---:|---:|\n")
	for _, c := range r.Cells {
		if c.Status == "not_run" {
			fmt.Fprintf(&report, "| %s | %s | %d | not run | — | — | — | — |\n", c.Task, c.Mode, c.Repetition)
		} else {
			fmt.Fprintf(&report, "| %s | %s | %d | %t | %d | %d | %.3f | %.3f |\n", c.Task, c.Mode, c.Repetition, *c.Success, *c.Actions, *c.ObservationBytes, *c.CaptureMS, *c.WallMS)
		}
	}
	must(t, os.WriteFile(filepath.Join(out, "README.md"), []byte(report.String()), 0644))
	if failed {
		t.Fatal("one or more scripted cells failed; see recorded results")
	}
}

// validate prevents absent adapters or unknown metrics from becoming successes.
func validate(cells []cell) error {
	seen := map[string]bool{}
	for _, c := range cells {
		key := fmt.Sprintf("%s/%s/%d", c.Task, c.Mode, c.Repetition)
		if seen[key] {
			return fmt.Errorf("duplicate cell %s", key)
		}
		seen[key] = true
		if c.ModelTokens != nil {
			return fmt.Errorf("scripted smoke cannot report model tokens")
		}
		switch c.Status {
		case "not_run":
			if c.Success != nil || c.Actions != nil || c.WallMS != nil || c.CaptureMS != nil || c.ObservationBytes != nil || c.Reason == "" {
				return fmt.Errorf("unrun cell has measurements or no reason")
			}
		case "completed", "failed":
			if c.Success == nil || c.Actions == nil || c.WallMS == nil || c.CaptureMS == nil || c.ObservationBytes == nil || c.Artifact == "" {
				return fmt.Errorf("measured cell lacks evidence")
			}
			if *c.Success != (c.Status == "completed") || *c.Actions < 0 || *c.WallMS < 0 || *c.CaptureMS < 0 || *c.ObservationBytes < 0 {
				return fmt.Errorf("inconsistent measured cell")
			}
		default:
			return fmt.Errorf("unknown status %q", c.Status)
		}
	}
	return nil
}
func TestRejectMisleadingResults(t *testing.T) {
	yes := true
	zero := 0
	tests := []cell{
		{Status: "not_run", Reason: "unavailable", Success: &yes},
		{Status: "not_run", Reason: "unavailable", ModelTokens: &zero},
		{Status: "completed", Success: &yes},
		{Status: "not_run"},
	}
	for _, c := range tests {
		if validate([]cell{c}) == nil {
			t.Errorf("accepted misleading record: %+v", c)
		}
	}
	if err := validate([]cell{{Status: "not_run", Reason: "unavailable"}}); err != nil {
		t.Fatal(err)
	}
}

// Committed evidence must agree with its metrics; timing is intentionally not a
// golden value, and successful fixture assertions are not model successes.
func TestRecordedEvidence(t *testing.T) {
	paths, err := filepath.Glob("../../evals/results/*/results.json")
	must(t, err)
	for _, path := range paths {
		b, e := os.ReadFile(path)
		must(t, e)
		var r result
		must(t, json.Unmarshal(b, &r))
		must(t, validate(r.Cells))
		if (r.RunComplete && len(r.Cells) != 36) || r.ExecutionKind != "scripted" || r.RunnerSHA == "" {
			t.Fatalf("incomplete provenance/matrix: %s", path)
		}
		for _, c := range r.Cells {
			if c.Status == "not_run" {
				continue
			}
			b, e := os.ReadFile(filepath.Join(filepath.Dir(path), c.Artifact))
			must(t, e)
			var events []event
			must(t, json.Unmarshal(b, &events))
			size, actions := 0, 0
			for _, event := range events {
				switch event.Kind {
				case "observation":
					size += len([]byte(event.Content))
				case "scripted_action":
					actions++
				}
			}
			if size != *c.ObservationBytes || actions != *c.Actions {
				t.Errorf("metrics differ from evidence: %s", c.Artifact)
			}
			if *c.Success && (len(events) < 2 || events[0].Kind != "precondition" || !strings.HasSuffix(events[0].Content, " => false") || events[len(events)-1].Kind != "oracle" || !strings.HasSuffix(events[len(events)-1].Content, " => true")) {
				t.Errorf("success lacks independent predicates: %s", c.Artifact)
			}
		}
	}
}
