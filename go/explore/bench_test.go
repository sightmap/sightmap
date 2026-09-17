package explore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSuiteResetsAndSummarises(t *testing.T) {
	dir := t.TempDir()
	suitePath := filepath.Join(dir, "suite.json")
	os.WriteFile(suitePath, []byte(`{
	  "name": "fake", "start_url": "https://s/", "reset": "clear-storage", "avoid": ["Delete"],
	  "goals": [
	    {"name": "login", "goal": "log in", "spec": {"done_when": {"view": "Inventory"}, "values": {"username": "u", "password": "p"}}},
	    {"name": "impossible", "goal": "x", "spec": {"done_when": {"view": "Nowhere"}}, "max_steps": 2}
	  ]}`), 0o644)
	suite, err := LoadSuite(suitePath)
	if err != nil {
		t.Fatal(err)
	}
	drv := loginSite()
	var out bytes.Buffer
	res, err := RunSuite(context.Background(), drv, suite, SuiteOptions{
		NewPicker: func() (Picker, error) {
			return &fakePicker{script: []string{"UsernameField", "PasswordField", "LoginButton"}}, nil
		},
		Out: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	// login: three actions plus the done step; impossible: two steps
	if res.Summary.Goals != 2 || res.Summary.OK != 1 || res.Summary.Steps != 6 {
		t.Fatalf("summary = %+v", res.Summary)
	}
	if !res.Runs[0].OK || res.Runs[1].OK || res.Runs[1].Reason != "no result within 2 steps" {
		t.Fatalf("runs = %+v / %+v", res.Runs[0].Run, res.Runs[1].Run)
	}
	// the suite's avoid list reaches every goal's spec
	if got := res.Runs[0].Spec.Avoid; len(got) != 1 || got[0] != "Delete" {
		t.Fatalf("avoid = %v", got)
	}
	// each goal started from the start URL again
	if drv.cur == "https://s/inventory.html" {
		t.Fatal("second goal did not reset")
	}
	table := FormatTable(res)
	for _, want := range []string{"login", "✓", "impossible", "✗", "fake: 1/2 ok · 6 steps"} {
		_ = want
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
	if !strings.Contains(out.String(), "=== login") || !strings.Contains(out.String(), "--> OK") {
		t.Errorf("progress output:\n%s", out.String())
	}
}

func TestLoadSuiteRejectsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, []byte(`{"name":"x","goals":[]}`), 0o644)
	if _, err := LoadSuite(p); err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatStep(t *testing.T) {
	s := FormatStep(Step{N: 3, View: "Inventory", Action: "clicked [CartLink]", Probs: "n54:0.91", DoneProb: 0.02, Ms: 812, MsSnap: 150, MsPick: 190, MsAct: 60, MsSettle: 400})
	if s != " 3. Inventory  clicked [CartLink]  [n54:0.91]  done=0.02  812ms (snap 150, pick 190, act 60, settle 400)" {
		t.Fatalf("got %q", s)
	}
}
