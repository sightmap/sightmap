package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// writeStatsCorpus writes a .sightmap/ corpus into a temp dir and returns its
// path, for driving runStatsOut end to end through the real loader.
func writeStatsCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// TestStatsJSON_PublishedFieldSet pins the --json contract consumed by CI in
// other repos (the atlas index generator): the exact top-level field set and
// the exact per-row field set, nothing added, nothing renamed, and per_view
// present as a list even when it is empty.
func TestStatsJSON_PublishedFieldSet(t *testing.T) {
	dir := writeStatsCorpus(t, map[string]string{
		"components.yaml": `version: 1
components:
  - name: Navigation
    selector: 'nav'
`,
		"views/home.yaml": `version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: Navigation
      - name: Hero
        selector: '.hero'
        properties:
          - name: headline
            extract: text
`,
	})

	var out bytes.Buffer
	if err := runStatsOut([]string{"--sightmap-dir", dir, "--json"}, &out); err != nil {
		t.Fatalf("runStatsOut: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal %q: %v", out.String(), err)
	}
	assertFieldSet(t, "top level", raw,
		[]string{"components", "memory", "per_view", "properties", "requests", "views"})

	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw["per_view"], &rows); err != nil {
		t.Fatalf("unmarshal per_view: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("per_view has %d rows, want 1", len(rows))
	}
	assertFieldSet(t, "per_view row", rows[0],
		[]string{"components", "name", "requests", "route"})

	// Values, so a refactor that keeps the shape but breaks the arithmetic is
	// caught here too.
	var got struct {
		Views      int `json:"views"`
		Components int `json:"components"`
		Requests   int `json:"requests"`
		Properties int `json:"properties"`
		Memory     int `json:"memory"`
		PerView    []struct {
			Name       string `json:"name"`
			Route      string `json:"route"`
			Components int    `json:"components"`
			Requests   int    `json:"requests"`
		} `json:"per_view"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal typed: %v", err)
	}
	if got.Views != 1 || got.Components != 2 || got.Requests != 0 || got.Properties != 1 || got.Memory != 0 {
		t.Errorf("totals = views:%d components:%d requests:%d properties:%d memory:%d, want 1/2/0/1/0",
			got.Views, got.Components, got.Requests, got.Properties, got.Memory)
	}
	if got.PerView[0].Name != "Home" || got.PerView[0].Route != "/" || got.PerView[0].Components != 2 {
		t.Errorf("per_view[0] = %+v, want Home / with 2 components", got.PerView[0])
	}
}

// assertFieldSet fails unless obj's keys are exactly want (sorted).
func assertFieldSet(t *testing.T, what string, obj map[string]json.RawMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(obj))
	for k := range obj {
		got = append(got, k)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s fields = %v, want exactly %v — this is a published contract", what, got, want)
	}
}

// TestStatsJSON_MemoryOnlyCorpus: a corpus of nothing but memory entries is
// legal (validate accepts it). --json must answer with the documented shape —
// zero counts and an empty per_view list — instead of the human empty-corpus
// error, which a consumer cannot parse.
func TestStatsJSON_MemoryOnlyCorpus(t *testing.T) {
	dir := writeStatsCorpus(t, map[string]string{
		"memory.yaml": `version: 1
memory:
  - the app is a single-page React app
`,
	})

	var out bytes.Buffer
	if err := runStatsOut([]string{"--sightmap-dir", dir, "--json"}, &out); err != nil {
		t.Fatalf("runStatsOut: %v", err)
	}
	if !strings.Contains(out.String(), `"per_view": []`) {
		t.Errorf("per_view should serialize as an empty list, got:\n%s", out.String())
	}
	var got struct {
		Views      int `json:"views"`
		Components int `json:"components"`
		Memory     int `json:"memory"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out.String(), err)
	}
	if got.Views != 0 || got.Components != 0 || got.Memory != 1 {
		t.Errorf("got views=%d components=%d memory=%d, want 0/0/1", got.Views, got.Components, got.Memory)
	}
}

// TestStats_EmptyCorpusTeachesSchema: an existing but wholly empty corpus fails
// with the schema example rather than printing an all-zero table.
func TestStats_EmptyCorpusTeachesSchema(t *testing.T) {
	dir := writeStatsCorpus(t, map[string]string{
		"components.yaml": "version: 1\n",
	})

	var out bytes.Buffer
	err := runStatsOut([]string{"--sightmap-dir", dir}, &out)
	if err == nil {
		t.Fatalf("runStatsOut succeeded on an empty corpus; output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "empty corpus") || !strings.Contains(err.Error(), "views:") {
		t.Errorf("error = %q, want an empty-corpus message that teaches the views: schema", err)
	}
	if out.Len() != 0 {
		t.Errorf("empty corpus printed a table:\n%s", out.String())
	}
}

// TestStats_GlobalsOnlyTable covers printStats' no-views branch: totals are
// printed, and the table is replaced by the line that explains why there are no
// rows.
func TestStats_GlobalsOnlyTable(t *testing.T) {
	dir := writeStatsCorpus(t, map[string]string{
		"components.yaml": `version: 1
components:
  - name: Header
    selector: '#header'
requests:
  - name: Ping
    route: /api/ping
`,
	})

	var out bytes.Buffer
	if err := runStatsOut([]string{"--sightmap-dir", dir}, &out); err != nil {
		t.Fatalf("runStatsOut: %v", err)
	}
	got := out.String()
	for _, want := range []string{"sightmap stats ·", "Views       0", "Components  1", "Requests    1",
		"no views defined — totals cover corpus-level definitions only"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "distinct components (per-view rows sum to") {
		t.Errorf("globals-only output should not print the per-view summary line:\n%s", got)
	}
}

// TestStats_RefusesCorpusWithValidationErrors: the loader drops an unresolved
// $ref and records an error-severity diagnostic, so the counts would silently
// under-report. stats must fail loudly instead of exiting zero.
func TestStats_RefusesCorpusWithValidationErrors(t *testing.T) {
	files := map[string]string{
		"components.yaml": `version: 1
components:
  - name: Navigation
    selector: 'nav'
`,
		"views/home.yaml": `version: 1
views:
  - name: Home
    route: /
    components:
      - $ref: Navigatoin
      - name: Hero
        selector: '.hero'
`,
	}

	t.Run("table mode", func(t *testing.T) {
		var out bytes.Buffer
		err := runStatsOut([]string{"--sightmap-dir", writeStatsCorpus(t, files)}, &out)
		if err == nil {
			t.Fatalf("runStatsOut succeeded on a corpus validate rejects; output:\n%s", out.String())
		}
		if !strings.Contains(err.Error(), "validation error") {
			t.Errorf("error = %q, want it to name the validation errors", err)
		}
		if out.Len() != 0 {
			t.Errorf("a table was printed for an uncountable corpus:\n%s", out.String())
		}
	})

	t.Run("json mode", func(t *testing.T) {
		var out bytes.Buffer
		err := runStatsOut([]string{"--sightmap-dir", writeStatsCorpus(t, files), "--json"}, &out)
		if err == nil {
			t.Fatalf("runStatsOut succeeded on a corpus validate rejects; output:\n%s", out.String())
		}
		var got struct {
			Error       string `json:"error"`
			Diagnostics []struct {
				Code      string `json:"code"`
				Component string `json:"component"`
				Message   string `json:"message"`
			} `json:"diagnostics"`
			Components *int `json:"components"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("--json failure must still be machine-readable; got %q: %v", out.String(), err)
		}
		if got.Error == "" {
			t.Error(`the failure envelope must carry a non-empty "error" key`)
		}
		if got.Components != nil {
			t.Errorf("a failure envelope must not carry counts, got components=%d", *got.Components)
		}
		if len(got.Diagnostics) == 0 {
			t.Fatalf("no diagnostics in %q", out.String())
		}
		if got.Diagnostics[0].Code != "ref-unresolved" || got.Diagnostics[0].Component != "Navigatoin" {
			t.Errorf("diagnostics[0] = %+v, want the unresolved $ref named", got.Diagnostics[0])
		}
	})
}

// TestStats_RejectsPositionalArgs: `sightmap stats json` is a typo for
// `stats --json`, not a request to print the table.
func TestStats_RejectsPositionalArgs(t *testing.T) {
	dir := writeStatsCorpus(t, map[string]string{
		"components.yaml": `version: 1
components:
  - name: Header
    selector: '#header'
`,
	})

	var out bytes.Buffer
	err := runStatsOut([]string{"--sightmap-dir", dir, "json"}, &out)
	if err == nil {
		t.Fatalf("runStatsOut accepted a stray positional argument; output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), `unexpected argument "json"`) {
		t.Errorf("error = %q, want it to name the unexpected argument", err)
	}
	if out.Len() != 0 {
		t.Errorf("output was printed despite the bad invocation:\n%s", out.String())
	}
}
