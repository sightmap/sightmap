package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildMCPArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    string
		params  []string
		want    map[string]any // compared after re-parsing (key order independent)
		wantErr bool
	}{
		{"empty defaults to object", "", nil, map[string]any{}, false},
		{"object passes through", `{"query":"ATL to LHR"}`, nil, map[string]any{"query": "ATL to LHR"}, false},
		{"array --args is rejected", `[1,2,3]`, nil, nil, true},
		{"scalar --args is rejected", `42`, nil, nil, true},
		{"malformed --args is rejected", `{not json`, nil, nil, true},
		{"param typed as JSON", "", []string{"count=3", "watched=true"}, map[string]any{"count": float64(3), "watched": true}, false},
		{"param falls back to string", "", []string{"query=ATL to LHR"}, map[string]any{"query": "ATL to LHR"}, false},
		{"param overrides --args", `{"q":"old","keep":1}`, []string{"q=new"}, map[string]any{"q": "new", "keep": float64(1)}, false},
		{"param not key=value is rejected", "", []string{"bogus"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildMCPArgs(tc.args, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("buildMCPArgs(%q,%v) = %q, nil; want error", tc.args, tc.params, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildMCPArgs(%q,%v) unexpected error: %v", tc.args, tc.params, err)
			}
			var gotObj map[string]any
			if err := json.Unmarshal([]byte(got), &gotObj); err != nil {
				t.Fatalf("result %q is not a JSON object: %v", got, err)
			}
			if len(gotObj) != len(tc.want) {
				t.Fatalf("buildMCPArgs = %v, want %v", gotObj, tc.want)
			}
			for k, wv := range tc.want {
				if gotObj[k] != wv {
					t.Errorf("key %q = %v (%T), want %v (%T)", k, gotObj[k], gotObj[k], wv, wv)
				}
			}
		})
	}
}

func TestMCPCallScript(t *testing.T) {
	s := mcpCallScript("search", `{"query":"ATL to LHR"}`)
	for _, want := range []string{
		"getTools", "executeTool", `const name = "search"`, `const args = {"query":"ATL to LHR"}`,
		// Native surfaces need args as a JSON string; the script must branch on it.
		"nativeSurface", "JSON.stringify(args)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("mcpCallScript missing %q in:\n%s", want, s)
		}
	}
	s2 := mcpCallScript(`a"b`, "{}")
	if !strings.Contains(s2, `const name = "a\"b"`) {
		t.Errorf("mcpCallScript did not escape the tool name safely:\n%s", s2)
	}
}

func TestRenderCallResult(t *testing.T) {
	// sightkick's polyfill shape: its ToolResult (ok/guidance) JSON-stringified
	// inside a CallToolResult text part. renderCallResult must surface it as
	// parsed JSON, not a stringified blob, and read isError.
	t.Run("unwraps a CallToolResult text envelope", func(t *testing.T) {
		inner := `{"ok":true,"guidance":"consider list_results next"}`
		env, _ := json.Marshal(map[string]any{
			"content": []map[string]any{{"type": "text", "text": inner}},
			"isError": false,
		})
		out, failed := renderCallResult(env, false)
		if failed {
			t.Error("failed = true, want false for isError:false")
		}
		if !strings.Contains(out, `"guidance": "consider list_results next"`) {
			t.Errorf("guidance not surfaced as parsed JSON; got:\n%s", out)
		}
		if strings.Contains(out, `\"guidance\"`) {
			t.Errorf("content was left stringified (escaped quotes) instead of unwrapped:\n%s", out)
		}
	})

	t.Run("isError envelope reports failure", func(t *testing.T) {
		env, _ := json.Marshal(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "boom"}},
			"isError": true,
		})
		out, failed := renderCallResult(env, false)
		if !failed {
			t.Error("failed = false, want true for isError:true")
		}
		if !strings.Contains(out, "boom") {
			t.Errorf("error text not surfaced; got:\n%s", out)
		}
	})

	t.Run("unwraps a native JSON-string-wrapped envelope", func(t *testing.T) {
		// Native Chrome returns the CallToolResult as a JSON string. renderCallResult
		// must unwrap that layer and still read isError + surface the content.
		innerEnv := `{"content":[{"type":"text","text":"nope"}],"isError":true}`
		wrapped, _ := json.Marshal(innerEnv) // a JSON string literal wrapping the envelope
		out, failed := renderCallResult(wrapped, false)
		if !failed {
			t.Error("failed = false, want true (isError inside the string-wrapped envelope)")
		}
		if !strings.Contains(out, "nope") {
			t.Errorf("content not surfaced from string-wrapped envelope; got:\n%s", out)
		}
	})

	t.Run("plain text string result prints as-is", func(t *testing.T) {
		out, failed := renderCallResult(json.RawMessage(`"just text"`), false)
		if failed || out != "just text" {
			t.Errorf("renderCallResult(plain string) = (%q,%v), want (\"just text\",false)", out, failed)
		}
	})

	t.Run("plain non-envelope value is pretty-printed", func(t *testing.T) {
		out, failed := renderCallResult(json.RawMessage(`{"a":1}`), false)
		if failed {
			t.Error("failed = true, want false")
		}
		if !strings.Contains(out, `"a": 1`) {
			t.Errorf("plain value not pretty-printed; got:\n%s", out)
		}
	})

	t.Run("--json passes the raw value through", func(t *testing.T) {
		raw := json.RawMessage(`{"content":[{"type":"text","text":"x"}],"isError":true}`)
		out, failed := renderCallResult(raw, true)
		if out != string(raw) {
			t.Errorf("--json out = %q, want raw %q", out, string(raw))
		}
		if !failed {
			t.Error("--json should still report isError failure")
		}
	})
}
