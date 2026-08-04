package sightmap_test

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

func messageCorpus(msgs ...sightmap.MessageDef) *sightmap.Corpus {
	return &sightmap.Corpus{Messages: msgs}
}

func TestValidate_MessageMissingName(t *testing.T) {
	errs := sightmap.Validate(messageCorpus(sightmap.MessageDef{Level: "ERROR"}))
	if !hasCode(errs, "missing-name") {
		t.Fatalf("want missing-name, got %v", findingCodes(errs))
	}
}

// A `message:` is an author-written regex. The corpus already compiles
// author-written component selectors at validation time; patterns get the same
// treatment rather than being stored unverified.
func TestValidate_MessageRegexInvalid(t *testing.T) {
	for _, pat := range []string{"(", "[a-", `a{2,1}`, `(?P<`} {
		errs := sightmap.Validate(messageCorpus(sightmap.MessageDef{
			Name:    "Bad",
			Message: pat,
		}))
		if !hasCode(errs, "message-regex-invalid") {
			t.Errorf("pattern %q: want message-regex-invalid, got %v", pat, findingCodes(errs))
		}
	}
}

func TestValidate_MessageRegexValidIsClean(t *testing.T) {
	errs := sightmap.Validate(messageCorpus(sightmap.MessageDef{
		Name:    "SlowNetworkWarning",
		Level:   "WARN",
		Message: `request .* took over \d+ms`,
	}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// Two messages sharing a name is the case that silently defeats a signals
// ref: resolution counts distinct entity kinds, so the duplicate collapses to
// one kind and neither unresolved nor ambiguous fires.
func TestValidate_MessageNameCollision(t *testing.T) {
	errs := sightmap.Validate(messageCorpus(
		sightmap.MessageDef{Name: "CartVersionMismatch", Level: "ERROR", Message: "a"},
		sightmap.MessageDef{Name: "CartVersionMismatch", Level: "WARN", Message: "b"},
	))
	var found *sightmap.ValidationError
	for i := range errs {
		if errs[i].Code == "merge-collision-message" {
			found = &errs[i]
		}
	}
	if found == nil {
		t.Fatalf("want merge-collision-message, got %v", findingCodes(errs))
	}
	if found.IsError() {
		t.Error("a name collision should be a warning, matching merge-collision-view")
	}
}

func TestValidate_MessageConflict(t *testing.T) {
	cases := []struct {
		name string
		a, b sightmap.MessageDef
		want bool
	}{
		{
			name: "same level, identical pattern",
			a:    sightmap.MessageDef{Name: "A", Level: "ERROR", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "boom"},
			want: true,
		},
		{
			name: "same level, one pattern absent",
			a:    sightmap.MessageDef{Name: "A", Level: "ERROR"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "boom"},
			want: true,
		},
		{
			name: "one level absent matches any",
			a:    sightmap.MessageDef{Name: "A", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "boom"},
			want: true,
		},
		{
			name: "level differs in case only",
			a:    sightmap.MessageDef{Name: "A", Level: "error", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "boom"},
			want: true,
		},
		{
			name: "different levels cannot overlap",
			a:    sightmap.MessageDef{Name: "A", Level: "ERROR", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "WARN", Message: "boom"},
			want: false,
		},
		{
			name: "different patterns are not statically decidable",
			a:    sightmap.MessageDef{Name: "A", Level: "ERROR", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "crash"},
			want: false,
		},
		{
			name: "exception does not overlap error",
			a:    sightmap.MessageDef{Name: "A", Level: "EXCEPTION", Message: "boom"},
			b:    sightmap.MessageDef{Name: "B", Level: "ERROR", Message: "boom"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := sightmap.Validate(messageCorpus(tc.a, tc.b))
			got := hasCode(errs, "message-conflict")
			if got != tc.want {
				t.Fatalf("message-conflict = %v, want %v (findings: %v)", got, tc.want, findingCodes(errs))
			}
		})
	}
}

// SEP-0006 permits an entry declaring neither constraint. It matches every
// record, which is legal and rarely useful, but not a diagnostic on its own.
func TestValidate_MessageNoConstraintsIsLegal(t *testing.T) {
	errs := sightmap.Validate(messageCorpus(sightmap.MessageDef{Name: "AnyRecord"}))
	if len(errs) != 0 {
		t.Fatalf("want no findings, got %v", findingCodes(errs))
	}
}

// level: WARNING is CDP's own spelling, normalized to warn by the capture, so it
// matches nothing. Advisory rather than an error because level is open.
func TestLint_MessageLevelUnknown(t *testing.T) {
	warns := sightmap.Lint(messageCorpus(
		sightmap.MessageDef{Name: "Typo", Level: "WARNING", Message: "x"},
		sightmap.MessageDef{Name: "Fine", Level: "EXCEPTION", Message: "y"},
	))
	var got []string
	for _, w := range warns {
		if w.Rule == "message-level-unknown" {
			got = append(got, w.Component)
		}
	}
	if len(got) != 1 || got[0] != "Typo" {
		t.Fatalf("want message-level-unknown on Typo only, got %v", got)
	}
}

func TestLoadDir_Messages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/app.yaml", `
version: 1
messages:
  - name: CartVersionMismatch
    level: ERROR
    message: cart version mismatch
    description: Cart mutated elsewhere
    source: src/cart/sync.ts
  - name: UncaughtCheckoutError
    level: EXCEPTION
`)
	c, err := sightmap.DirLoader(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(c.Messages))
	}
	got := c.Messages[0]
	want := sightmap.MessageDef{
		Name:        "CartVersionMismatch",
		Level:       "ERROR",
		Message:     "cart version mismatch",
		Description: "Cart mutated elsewhere",
		Source:      "src/cart/sync.ts",
	}
	if got != want {
		t.Errorf("messages[0] = %+v, want %+v", got, want)
	}
	// The declared level is preserved verbatim; case-insensitivity applies when
	// matching a record, not at load.
	if c.Messages[1].Level != "EXCEPTION" {
		t.Errorf("declared level should round-trip verbatim, got %q", c.Messages[1].Level)
	}
	if !strings.EqualFold(c.Messages[1].Level, sightmap.LevelException) {
		t.Errorf("level %q should case-insensitively equal %q", c.Messages[1].Level, sightmap.LevelException)
	}
}
