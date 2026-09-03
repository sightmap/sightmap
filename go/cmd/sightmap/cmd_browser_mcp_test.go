package main

import (
	"strings"
	"testing"
)

func TestNormalizeMCPArgs(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty defaults to object", "", "{}", false},
		{"object passes through", `{"query":"ATL to LHR"}`, `{"query":"ATL to LHR"}`, false},
		{"array is rejected", `[1,2,3]`, "", true},
		{"scalar is rejected", `42`, "", true},
		{"malformed json is rejected", `{not json`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeMCPArgs(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeMCPArgs(%q) = %q, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeMCPArgs(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("normalizeMCPArgs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMCPCallScript(t *testing.T) {
	s := mcpCallScript("search", `{"query":"ATL to LHR"}`)
	// Drives the standard surface and injects the args object verbatim.
	for _, want := range []string{"getTools", "executeTool", `const name = "search"`, `{"query":"ATL to LHR"}`} {
		if !strings.Contains(s, want) {
			t.Errorf("mcpCallScript missing %q in:\n%s", want, s)
		}
	}

	// A tool name with a quote must be a safe JS string literal, not a break-out.
	s2 := mcpCallScript(`a"b`, "{}")
	if !strings.Contains(s2, `const name = "a\"b"`) {
		t.Errorf("mcpCallScript did not escape the tool name safely:\n%s", s2)
	}
}
