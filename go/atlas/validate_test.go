package atlas

import (
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		name string
		slug string
		ok   bool
	}{
		{"plain", "square-pos", true},
		{"dotted", "acme.shop_2", true},
		{"unicode", "café-demo", true},
		{"empty", "", false},
		{"slash", "a/b", false},
		{"backslash", `a\b`, false},
		{"traversal", "..", false},
		{"hidden-traversal", "a..b", false},
		{"escape", "shop\x1b[31m", false},
		{"newline", "shop\nInstalled evil", false},
		{"tab", "shop\tx", false},
		{"del", "shop\x7f", false},
		{"c1-control", "shop\u0085", false},
		{"invalid-utf8", "shop\xff", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSlug(tc.slug)
			if tc.ok && err != nil {
				t.Fatalf("ValidateSlug(%q) = %v, want ok", tc.slug, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateSlug(%q) = nil, want an error", tc.slug)
			}
		})
	}
}

func TestValidateCommit(t *testing.T) {
	cases := []struct {
		name   string
		commit string
		ok     bool
	}{
		{"empty-is-unpinned", "", true},
		{"sha", "0123456789abcdef0123456789abcdef01234567", true},
		{"uppercase", "0123456789ABCDEF0123456789abcdef01234567", false},
		{"short", "0123456", false},
		{"not-hex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", false},
		{"branch", "main", false},
		{"path-injection", "../../../../etc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCommit(tc.commit)
			if tc.ok != (err == nil) {
				t.Fatalf("ValidateCommit(%q) = %v, ok=%v", tc.commit, err, tc.ok)
			}
		})
	}
}

func TestValidateCorpusPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		ok   bool
	}{
		{"config", ".sightmap/config.yaml", true},
		{"nested", ".sightmap/views/checkout.yaml", true},
		{"empty", "", false},
		{"traversal", ".sightmap/../evil.yaml", false},
		{"absolute", "/etc/passwd", false},
		{"backslash", `.sightmap\evil.yaml`, false},
		{"outside-corpus", "README.md", false},
		{"empty-segment", ".sightmap//x.yaml", false},
		{"dot-segment", ".sightmap/./x.yaml", false},
		{"trailing-slash", ".sightmap/views/", false},
		{"escape-in-filename", ".sightmap/\x1b]0;pwned\x07.yaml", false},
		{"newline-in-filename", ".sightmap/a\nb.yaml", false},
		{"invalid-utf8", ".sightmap/\xffx.yaml", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCorpusPath(tc.path)
			if tc.ok != (err == nil) {
				t.Fatalf("ValidateCorpusPath(%q) = %v, ok=%v", tc.path, err, tc.ok)
			}
		})
	}
}

func TestEntryValidate(t *testing.T) {
	good := Entry{Slug: "shop", Files: []string{".sightmap/config.yaml"}}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}

	tooMany := Entry{Slug: "shop"}
	for i := 0; i <= MaxEntryFiles; i++ {
		tooMany.Files = append(tooMany.Files, ".sightmap/config.yaml")
	}
	err := tooMany.Validate()
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("entry with %d files: got %v, want a file-count limit error", len(tooMany.Files), err)
	}

	noFiles := Entry{Slug: "shop"}
	if err := noFiles.Validate(); err == nil {
		t.Error("entry with no files should be rejected")
	}
}

func TestSafeText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "square-pos", "square-pos"},
		{"unicode-kept", "Café POS →", "Café POS →"},
		{"escape", "\x1b[31mred", `\x1b[31mred`},
		{"title-sequence", "\x1b]0;pwned\x07", `\x1b]0;pwned\x07`},
		{"newline", "a\nb", `a\x0ab`},
		{"carriage-return", "a\rInstalled everything", `a\x0dInstalled everything`},
		{"del", "a\x7f", `a\x7f`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SafeText(tc.in); got != tc.want {
				t.Errorf("SafeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	long := strings.Repeat("x", 500)
	if got := SafeText(long); len([]rune(got)) != maxSafeTextRunes+1 {
		t.Errorf("SafeText truncation: got %d runes, want %d", len([]rune(got)), maxSafeTextRunes+1)
	}
}
