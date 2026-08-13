package sightmap_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// The spec conformance fixtures under spec/conformance/*.fixture each carry an
// expected.json whose `cases` are executed by almost nothing: the JS runner
// (spec/scripts/validate-sightmap.mjs) only schema-validates the YAML, and the
// Go match runner (go/match/conformance_test.go) reads a different corpus under
// go/conformance/fixtures. So a fixture could assert diagnostics the reference
// validator never produces and still stay green.
//
// runSpecValidateFixtures closes that gap for `validate` cases: it runs the
// reference validator (the same sightmap.Validate the `validate` command calls)
// over each fixture matching glob and asserts its `validate` case's diagnostics
// exactly. It is scoped by glob on purpose — a whole-suite executor also needs
// the match/lint/explain commands, and at least one existing fixture (017-tags)
// currently diverges from its own expected.json, so a full executor is separate
// work.
func TestSpecConformance_RequestPropertyFixtures(t *testing.T) {
	runSpecValidateFixtures(t, "*-request-properties.fixture")
}

func TestSpecConformance_MessageFixtures(t *testing.T) {
	runSpecValidateFixtures(t, "*-messages.fixture")
}

func TestSpecConformance_MessagePropertyFixtures(t *testing.T) {
	runSpecValidateFixtures(t, "*-message-properties.fixture")
}

func runSpecValidateFixtures(t *testing.T, glob string) {
	t.Helper()
	const root = "../../spec/conformance"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("spec conformance fixtures not reachable from the module (%v)", err)
	}
	fixtures, err := filepath.Glob(filepath.Join(root, glob))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("no fixtures matched %q; did a fixture move or get renamed?", glob)
	}
	for _, dir := range fixtures {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			corpus, err := sightmap.Load(filepath.Join(dir, "sightmap"))
			if err != nil {
				t.Fatalf("load corpus: %v", err)
			}
			got := validateDiagnostics(corpus)
			want := expectedValidateDiagnostics(t, filepath.Join(dir, "expected.json"))

			sortDiags(got)
			sortDiags(want)
			if len(got) != len(want) {
				t.Fatalf("validate diagnostics do not match expected.json\n got: %v\nwant: %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("validate diagnostics do not match expected.json\n got: %v\nwant: %v", got, want)
				}
			}
		})
	}
}

type specDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
}

// validateDiagnostics runs the checks the `validate` command runs and returns
// each finding as a {code, severity} pair. Validate() is the whole story for a
// property/message corpus (the command's extra "no-views" nudge only fires for
// a corpus with global components).
func validateDiagnostics(c *sightmap.Corpus) []specDiag {
	var out []specDiag
	for _, f := range sightmap.Validate(c) {
		sev := "warning"
		if f.IsError() {
			sev = "error"
		}
		out = append(out, specDiag{Code: f.Code, Severity: sev})
	}
	return out
}

func expectedValidateDiagnostics(t *testing.T, path string) []specDiag {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read expected.json: %v", err)
	}
	var doc struct {
		Cases []struct {
			Command  string `json:"command"`
			Expected struct {
				Diagnostics []specDiag `json:"diagnostics"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse expected.json: %v", err)
	}
	for _, c := range doc.Cases {
		if c.Command == "validate" {
			return c.Expected.Diagnostics
		}
	}
	t.Fatalf("expected.json has no validate case")
	return nil
}

func sortDiags(d []specDiag) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Code != d[j].Code {
			return d[i].Code < d[j].Code
		}
		return d[i].Severity < d[j].Severity
	})
}
