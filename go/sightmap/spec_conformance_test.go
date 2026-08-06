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
// expected.json whose `cases` were, until now, executed by nothing: the JS
// runner (spec/scripts/validate-sightmap.mjs) only schema-validates the YAML,
// and the Go match runner (go/match/conformance_test.go) reads a different
// corpus under go/conformance/fixtures. So a fixture could assert diagnostics
// the reference validator never produces and still stay green — which is
// exactly what 019-messages did before this test (it asserted zero diagnostics
// while `sightmap validate` emitted a message-conflict warning).
//
// This test closes that gap for the message fixtures: it runs the reference
// validator (the same sightmap.Validate the `validate` command calls) over each
// *-messages fixture and asserts the resulting diagnostics match the fixture's
// `validate` case exactly. It is intentionally scoped to message fixtures —
// executing every fixture also needs the match/lint/explain commands, and at
// least one existing fixture (017-tags) currently diverges from its own
// expected.json, so a whole-suite executor is separate, larger work.
func TestSpecConformance_MessageFixtures(t *testing.T) {
	const root = "../../spec/conformance"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("spec conformance fixtures not reachable from the module (%v)", err)
	}
	fixtures, err := filepath.Glob(filepath.Join(root, "*-messages.fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no *-messages.fixture found; did the fixture move or get renamed?")
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
// each finding as a {code, severity} pair. It mirrors runValidate: Validate()
// is the whole story for a message corpus (the command's extra "no-views" nudge
// only fires for a corpus with global components, which a messages: fixture has
// none of).
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
