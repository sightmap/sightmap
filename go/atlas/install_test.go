package atlas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAtlas serves an atlas under the documented layout: every body is keyed
// by its full URL path, so a test spells out exactly which URLs exist.
type fakeAtlas struct {
	*httptest.Server
	mu sync.Mutex
	// requested records every path served, in order.
	requested []string
	// before, when set, runs before a body is written — a hook for racing the
	// install.
	before func(path string)
	// delay, when set, is slept before each body is written.
	delay time.Duration
	// inFlight tracks concurrency.
	inFlight, maxInFlight int
}

func newFakeAtlas(t *testing.T, files map[string]string) *fakeAtlas {
	t.Helper()
	f := &fakeAtlas{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requested = append(f.requested, r.URL.Path)
		f.inFlight++
		if f.inFlight > f.maxInFlight {
			f.maxInFlight = f.inFlight
		}
		before, delay := f.before, f.delay
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.inFlight--
			f.mu.Unlock()
		}()

		if before != nil {
			before(r.URL.Path)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAtlas) indexURL() string { return f.URL + "/main/index.json" }

func (f *fakeAtlas) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requested...)
}

// standardIndex carries unknown fields (generated_at, stars, description) on
// purpose — install must ignore them, not choke on them.
const standardIndex = `{
  "schema_version": 1,
  "generated_at": "2026-08-01T00:00:00Z",
  "entries": [
    {
      "slug": "square-pos",
      "name": "Square POS",
      "description": "point of sale demo",
      "stars": 12,
      "files": [".sightmap/config.yaml", ".sightmap/views/checkout.yaml"]
    },
    {
      "slug": "pinned-shop",
      "name": "Pinned Shop",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "files": [".sightmap/config.yaml"]
    },
    {
      "slug": "square-web",
      "files": [".sightmap/config.yaml"]
    }
  ]
}`

const (
	configYAML = "version: 1\n"
	viewYAML   = "version: 1\nviews: []\n"
)

func standardFiles() map[string]string {
	return map[string]string{
		"/main/index.json": standardIndex,
		"/main/entries/square-pos/.sightmap/config.yaml":                                      configYAML,
		"/main/entries/square-pos/.sightmap/views/checkout.yaml":                              viewYAML,
		"/main/entries/square-web/.sightmap/config.yaml":                                      configYAML,
		"/0123456789abcdef0123456789abcdef01234567/entries/pinned-shop/.sightmap/config.yaml": configYAML,
	}
}

func install(t *testing.T, slug string, opts Options) (*Result, error) {
	t.Helper()
	return Install(context.Background(), slug, opts)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// ── happy path ────────────────────────────────────────────────────────────────

func TestInstall_writesTheCorpus(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q", got)
	}
	if got := readFile(t, filepath.Join(target, "views", "checkout.yaml")); got != viewYAML {
		t.Errorf("views/checkout.yaml = %q", got)
	}
	// The .sightmap/ prefix is the wire format's, not the target's.
	if want := []string{"config.yaml", "views/checkout.yaml"}; fmt.Sprint(res.Files) != fmt.Sprint(want) {
		t.Errorf("Files = %v, want %v", res.Files, want)
	}
	if res.Slug != "square-pos" || res.Name != "Square POS" || res.Label() != "square-pos (Square POS)" {
		t.Errorf("result identity = %+v, label %q", res, res.Label())
	}
	if res.Replaced {
		t.Error("Replaced should be false for a fresh install")
	}
}

// ── pinning and refs ──────────────────────────────────────────────────────────

func TestInstall_pinnedEntryFetchesAtItsCommit(t *testing.T) {
	// The file exists ONLY under the entry's commit, so a success proves the
	// pinned sha was used as the ref.
	srv := newFakeAtlas(t, standardFiles())
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "pinned-shop", Options{IndexURL: srv.indexURL(), Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Pinned || res.Ref != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("Ref = %q, Pinned = %v", res.Ref, res.Pinned)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("a pinned entry should not warn: %v", res.Warnings)
	}
}

// An index read from a non-main ref must fetch that ref's files. Hardcoding
// "main" 404s new entries and silently installs stale content for changed ones.
func TestInstall_unpinnedEntryFollowsTheIndexRef(t *testing.T) {
	srv := newFakeAtlas(t, map[string]string{
		"/next/index.json": standardIndex,
		"/next/entries/square-web/.sightmap/config.yaml": configYAML,
		// The same file also exists on main with different content; picking it
		// up would be the bug.
		"/main/entries/square-web/.sightmap/config.yaml": "version: 1\n# stale\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "square-web", Options{IndexURL: srv.URL + "/next/index.json", Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Ref != "next" {
		t.Errorf("Ref = %q, want %q", res.Ref, "next")
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("installed the wrong ref's content: %q", got)
	}
}

// An unpinned entry can straddle an atlas push: the index and the files are
// separate reads of a moving ref. That is not refusable, but it is reportable.
func TestInstall_unpinnedEntryWarns(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning about the floating ref")
	}
	for _, want := range []string{"no commit", "main"} {
		if !strings.Contains(res.Warnings[0], want) {
			t.Errorf("warning = %q, want it to mention %q", res.Warnings[0], want)
		}
	}
}

// ── local preconditions ───────────────────────────────────────────────────────

// A user with an existing corpus and no network should get the actionable
// refusal, not a 30-second dial timeout.
func TestInstall_refusesNonEmptyTargetBeforeAnyNetworkIO(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "hand-authored.yaml"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: target})
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("error = %v, want ErrTargetNotEmpty", err)
	}
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted before the local check: %v", paths)
	}
	if got := readFile(t, filepath.Join(target, "hand-authored.yaml")); got != "precious\n" {
		t.Errorf("existing file was touched: %q", got)
	}
}

func TestInstall_allowsAnEmptyExistingTarget(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := t.TempDir() // exists, empty

	if _, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q", got)
	}
}

// An empty target is a bug, not "the current directory": os.ReadDir("") fails
// with IsNotExist, which would read as "target absent" and write the corpus
// over whatever is in the working directory.
func TestInstall_rejectsEmptyTarget(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: ""})
	if err == nil {
		t.Fatal("expected an error for an empty target")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("error = %v, want it to name the target", err)
	}
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted for an unusable target: %v", paths)
	}
}

// ── replace, not merge ────────────────────────────────────────────────────────

func TestInstall_replaceReplacesTheWholeTarget(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(filepath.Join(target, "views"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"config.yaml":       "version: 1\n# old\n",
		"views/stale.yaml":  "version: 1\nviews: []\n",
		"views/nested.yaml": "version: 1\nviews: []\n",
	} {
		if err := os.WriteFile(filepath.Join(target, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Replaced {
		t.Error("Replaced = false, want true")
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q, want the fetched content", got)
	}
	// Files the entry does not publish must not survive: a hybrid corpus loads
	// stale views long after the install printed success.
	for _, stale := range []string{"views/stale.yaml", "views/nested.yaml"} {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(stale))); !os.IsNotExist(err) {
			t.Errorf("%s survived a --force install (stat err = %v)", stale, err)
		}
	}
	// The staging directory is cleaned up, whatever happened.
	assertNoLeftovers(t, filepath.Dir(target))
}

// ── --force guardrails ────────────────────────────────────────────────────────

// projectFiles is a directory that is plainly not a corpus: a git checkout with
// secrets, a manifest, and source. Replacing it would delete work no atlas
// entry can put back.
func projectFiles() map[string]string {
	return map[string]string{
		".git/HEAD":   "ref: refs/heads/main\n",
		".env":        "STRIPE_KEY=sk_live_xxx\n",
		"go.mod":      "module example.com/app\n",
		"src/main.go": "package main\n",
		"config.yaml": "version: 1\n",
	}
}

// assertIntact re-reads every file of a layout and fails on the first one that
// changed or vanished.
func assertIntact(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, want := range files {
		if want == "" && strings.HasSuffix(p, "/") {
			continue
		}
		if got := readFile(t, filepath.Join(root, filepath.FromSlash(p))); got != want {
			t.Errorf("%s = %q, want %q — the refused install deleted or rewrote it", p, got, want)
		}
	}
}

// The reported destruction: `--target . --force` in a git repo renamed the
// whole checkout aside and RemoveAll'd it, exit 0.
func TestInstall_forceRefusesAProjectDirectory(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := t.TempDir()
	files := projectFiles()
	writeTree(t, target, files)

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("error = %v, want ErrUnsafeReplace", err)
	}
	for _, want := range []string{".git", ".env", target} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %q", err, want)
		}
	}
	assertIntact(t, target, files)
	// The refusal is a local precondition, so it holds with or without
	// connectivity — and nothing is fetched for an install that cannot land.
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted before the local refusal: %v", paths)
	}
}

// `--target .` — the resolved target is the working directory. The directory
// here is corpus-shaped, so only the location rule can be refusing it.
func TestInstall_forceRefusesTheWorkingDirectory(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := t.TempDir()
	files := map[string]string{"config.yaml": "version: 1\n# mine\n", "views/mine.yaml": viewYAML}
	writeTree(t, target, files)
	chdir(t, target)

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: ".", Replace: true})
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("error = %v, want ErrUnsafeReplace", err)
	}
	if !strings.Contains(err.Error(), "current working directory") {
		t.Errorf("error = %v, want it to name the working directory", err)
	}
	assertIntact(t, target, files)
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted before the local refusal: %v", paths)
	}
}

// `--target ..` from inside the corpus.
func TestInstall_forceRefusesAnAncestorOfTheWorkingDirectory(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	target := t.TempDir()
	files := map[string]string{"config.yaml": "version: 1\n# mine\n", "views/mine.yaml": viewYAML}
	writeTree(t, target, files)
	chdir(t, filepath.Join(target, "views"))

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: "..", Replace: true})
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("error = %v, want ErrUnsafeReplace", err)
	}
	if !strings.Contains(err.Error(), "contains the current working directory") {
		t.Errorf("error = %v, want it to name the working directory it contains", err)
	}
	assertIntact(t, target, files)
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted before the local refusal: %v", paths)
	}
}

// The emptiness check runs before a multi-second fetch, so what --force
// destroys has to be judged again at the swap — on what the target holds now.
func TestInstall_forceRefusesAProjectThatAppearedDuringTheFetch(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	files := projectFiles()
	srv.before = func(path string) {
		if strings.HasSuffix(path, "config.yaml") {
			writeTree(t, target, files)
		}
	}

	_, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("error = %v, want ErrUnsafeReplace", err)
	}
	assertIntact(t, target, files)
	assertNoLeftovers(t, parent)
}

// The feature --force exists for: a real corpus directory, replaced wholesale.
func TestInstall_forceStillReplacesACorpusDirectory(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	writeTree(t, target, map[string]string{
		"config.yaml":          "version: 1\n# old\n",
		"components.yaml":      "version: 1\n",
		"views/stale.yaml":     viewYAML,
		"snapshots/plp/a.snap": "{}",
		"review/notes.yaml":    "- look at the header\n",
		".DS_Store":            "\x00\x00finder\n",
	})

	res, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if err != nil {
		t.Fatalf("a corpus directory was refused: %v", err)
	}
	if !res.Replaced {
		t.Error("Replaced = false, want true")
	}
	if got := readFile(t, filepath.Join(target, "config.yaml")); got != configYAML {
		t.Errorf("config.yaml = %q, want the fetched content", got)
	}
	for _, gone := range []string{"components.yaml", "views/stale.yaml", "snapshots/plp/a.snap", "review/notes.yaml", ".DS_Store"} {
		if _, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(gone))); !os.IsNotExist(statErr) {
			t.Errorf("%s survived a --force install (stat err = %v)", gone, statErr)
		}
	}
	// The backup of the previous contents is discarded once the installed
	// corpus loads — success must not leave a copy of it beside the target.
	assertNoLeftovers(t, parent)
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != ".sightmap" {
		t.Errorf("%s holds %d entries after a successful install, want only the target", parent, len(entries))
	}
}

// ── rollback ──────────────────────────────────────────────────────────────────

// The load check is a gate, not a report: what the target held is kept until
// the corpus that replaced it loads. Otherwise a broken atlas entry costs the
// user their corpus *and* leaves them with one that does not work.
func TestInstall_forceRestoresThePreviousCorpusWhenTheInstalledOneDoesNotLoad(t *testing.T) {
	srv := newFakeAtlas(t, map[string]string{
		"/main/index.json": `{"schema_version": 1, "entries": [{"slug": "broken", "files": [".sightmap/views/checkout.yaml"]}]}`,
		"/main/entries/broken/.sightmap/views/checkout.yaml": "views:\n\t- name: bad tabs\n",
	})
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	files := map[string]string{
		"config.yaml":     "version: 1\n# mine\n",
		"views/mine.yaml": viewYAML,
	}
	writeTree(t, target, files)

	_, err := install(t, "broken", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if err == nil {
		t.Fatal("expected a load failure")
	}
	for _, want := range []string{"broken", "does not load", "were restored"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	// "nothing was installed" would be a lie here: something was, and undone.
	if strings.Contains(err.Error(), "nothing was installed") {
		t.Errorf("error = %v, want it to say the previous corpus was restored", err)
	}
	assertIntact(t, target, files)
	if _, statErr := os.Stat(filepath.Join(target, "views", "checkout.yaml")); !os.IsNotExist(statErr) {
		t.Error("the broken entry's file survived the rollback")
	}
	// The restore moves the backup back; nothing may be left beside the target.
	assertNoLeftovers(t, parent)
}

func assertNoLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".sightmap-add-") {
			t.Errorf("staging leftover %s in %s", e.Name(), dir)
		}
	}
}

// ── unsafe entries fail closed ────────────────────────────────────────────────

func TestInstall_refusesUnsafeEntries(t *testing.T) {
	cases := []struct{ name, entryJSON string }{
		{"traversal", `{"slug": "bad", "files": [".sightmap/../evil.yaml"]}`},
		{"absolute", `{"slug": "bad", "files": ["/etc/passwd"]}`},
		{"backslash", `{"slug": "bad", "files": [".sightmap\\evil.yaml"]}`},
		{"outside-corpus", `{"slug": "bad", "files": ["README.md"]}`},
		{"empty-segment", `{"slug": "bad", "files": [".sightmap//x.yaml"]}`},
		{"escape-in-path", `{"slug": "bad", "files": [".sightmap/\u001b]0;pwn\u0007.yaml"]}`},
		{"malformed-commit", `{"slug": "bad", "commit": "not-a-sha", "files": [".sightmap/config.yaml"]}`},
		{"no-files", `{"slug": "bad", "files": []}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := fmt.Sprintf(`{"schema_version": 1, "entries": [%s]}`, tc.entryJSON)
			srv := newFakeAtlas(t, map[string]string{"/main/index.json": index})
			target := filepath.Join(t.TempDir(), ".sightmap")

			_, err := Install(context.Background(), "bad", Options{IndexURL: srv.indexURL(), Target: target})
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Errorf("target %s should not exist after a rejected entry", target)
			}
			assertNoRawEscapes(t, err.Error())
		})
	}
}

// The slug is the user's own string, so it is checked before the atlas is
// contacted — an unusable one must be blamed on the argument, not on an entry.
func TestInstall_rejectsUnusableRequestedSlug(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	for _, slug := range []string{"", "square/../pos", `square\pos`, "square\x1b[2J"} {
		_, err := install(t, slug, Options{IndexURL: srv.indexURL(), Target: filepath.Join(t.TempDir(), ".sightmap")})
		if err == nil {
			t.Fatalf("slug %q was accepted", slug)
		}
		if !strings.Contains(err.Error(), "invalid slug") {
			t.Errorf("slug %q: error = %v, want it to blame the slug argument", slug, err)
		}
		assertNoRawEscapes(t, err.Error())
	}
	if paths := srv.paths(); len(paths) != 0 {
		t.Errorf("the atlas was contacted for an unusable slug: %v", paths)
	}
}

// Index-controlled strings reach a terminal. An ESC byte there rewrites the
// user's screen (or their title bar) — the vector git and npm both patched.
func assertNoRawEscapes(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		if isControl(r) && r != '\n' {
			t.Errorf("raw control character %q in user-facing text: %q", r, s)
			return
		}
	}
}

func TestInstall_sanitizesTheEntryNameInTheLabel(t *testing.T) {
	index := `{"schema_version": 1, "entries": [
	  {"slug": "shop", "name": "Shop\u001b[2J\u001b]0;pwned\u0007", "files": [".sightmap/config.yaml"]}
	]}`
	srv := newFakeAtlas(t, map[string]string{
		"/main/index.json":                         index,
		"/main/entries/shop/.sightmap/config.yaml": configYAML,
	})
	res, err := install(t, "shop", Options{IndexURL: srv.indexURL(), Target: filepath.Join(t.TempDir(), ".sightmap")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoRawEscapes(t, res.Label())
	if !strings.Contains(res.Label(), `\x1b`) {
		t.Errorf("label = %q, want the escape rendered visibly", res.Label())
	}
}

// The suggestion path fires *before* any entry is validated, so its strings are
// wholly unvalidated index input.
func TestInstall_slugMissSuggestsSafely(t *testing.T) {
	index := `{"schema_version": 1, "entries": [
	  {"slug": "square-pos", "files": [".sightmap/config.yaml"]},
	  {"slug": "square-web", "files": [".sightmap/config.yaml"]},
	  {"slug": "square\u001b[2J-evil", "files": [".sightmap/config.yaml"]},
	  {"slug": "pinned-shop", "files": [".sightmap/config.yaml"]}
	]}`
	srv := newFakeAtlas(t, map[string]string{"/main/index.json": index})

	_, err := install(t, "square", Options{IndexURL: srv.indexURL(), Target: filepath.Join(t.TempDir(), ".sightmap")})
	if err == nil {
		t.Fatal("expected an unknown-slug error")
	}
	assertNoRawEscapes(t, err.Error())
	for _, want := range []string{`"square"`, "square-pos", "square-web"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "pinned-shop") {
		t.Errorf("unrelated slug suggested: %v", err)
	}
}

func TestInstall_rejectsTooManyFiles(t *testing.T) {
	paths := make([]string, 0, MaxEntryFiles+1)
	for i := 0; i <= MaxEntryFiles; i++ {
		paths = append(paths, fmt.Sprintf("%q", fmt.Sprintf(".sightmap/views/v%d.yaml", i)))
	}
	index := fmt.Sprintf(`{"schema_version": 1, "entries": [{"slug": "huge", "files": [%s]}]}`, strings.Join(paths, ","))
	srv := newFakeAtlas(t, map[string]string{"/main/index.json": index})
	target := filepath.Join(t.TempDir(), ".sightmap")

	_, err := install(t, "huge", Options{IndexURL: srv.indexURL(), Target: target})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("error = %v, want a file-count limit refusal", err)
	}
	// The refusal must precede the fetching, not follow 257 round trips.
	if paths := srv.paths(); len(paths) != 1 {
		t.Errorf("fetched %d URLs before refusing, want only the index", len(paths))
	}
}

// ── schema gate ───────────────────────────────────────────────────────────────

func TestInstall_rejectsNewerSchemaVersion(t *testing.T) {
	index := `{"schema_version": 2, "entries": {"square-pos": {"files": [".sightmap/config.yaml"]}}}`
	srv := newFakeAtlas(t, map[string]string{"/main/index.json": index})
	target := filepath.Join(t.TempDir(), ".sightmap")

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: target})
	if err == nil {
		t.Fatal("expected a schema_version refusal")
	}
	for _, want := range []string{"schema_version", "upgrade sightmap", srv.indexURL()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

// ── atomicity ─────────────────────────────────────────────────────────────────

func TestInstall_fetchFailureWritesNothing(t *testing.T) {
	files := standardFiles()
	delete(files, "/main/entries/square-pos/.sightmap/views/checkout.yaml")
	srv := newFakeAtlas(t, files)
	target := filepath.Join(t.TempDir(), ".sightmap")

	_, err := install(t, "square-pos", Options{IndexURL: srv.indexURL(), Target: target})
	if err == nil {
		t.Fatal("expected an error for a missing corpus file")
	}
	for _, want := range []string{"/main/entries/square-pos/.sightmap/views/checkout.yaml", "404"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("target %s should not exist after a failed fetch", target)
	}
}

// Fetch-all-before-write only covers network failure. A write that fails
// halfway — here an entry whose own paths collide, a plain file and a
// directory with the same name — must not leave the earlier files behind.
func TestInstall_writeFailureLeavesTargetUntouched(t *testing.T) {
	index := `{"schema_version": 1, "entries": [{"slug": "colliding", "files": [
	  ".sightmap/config.yaml", ".sightmap/views", ".sightmap/views/checkout.yaml"]}]}`
	srv := newFakeAtlas(t, map[string]string{
		"/main/index.json": index,
		"/main/entries/colliding/.sightmap/config.yaml":         configYAML,
		"/main/entries/colliding/.sightmap/views":               "not a directory\n",
		"/main/entries/colliding/.sightmap/views/checkout.yaml": viewYAML,
	})
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "hand-authored.yaml"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := install(t, "colliding", Options{IndexURL: srv.indexURL(), Target: target, Replace: true})
	if err == nil {
		t.Fatal("expected a write failure")
	}
	if got := readFile(t, filepath.Join(target, "hand-authored.yaml")); got != "precious\n" {
		t.Errorf("the previous corpus was damaged: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(target, "config.yaml")); !os.IsNotExist(statErr) {
		t.Error("a partially-written file survived the failure")
	}
	assertNoLeftovers(t, parent)
}

// The emptiness check happens before a multi-second fetch. A file that appears
// during the fetch must not be silently overwritten.
func TestInstall_targetFilledDuringFetchIsNotOverwritten(t *testing.T) {
	srv := newFakeAtlas(t, standardFiles())
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	srv.before = func(path string) {
		if strings.HasSuffix(path, "config.yaml") {
			_ = os.WriteFile(filepath.Join(target, "hand-authored.yaml"), []byte("precious\n"), 0o644)
		}
	}

	_, err := install(t, "square-web", Options{IndexURL: srv.indexURL(), Target: target})
	if err == nil {
		t.Fatal("expected the install to refuse a target that filled up mid-fetch")
	}
	if got := readFile(t, filepath.Join(target, "hand-authored.yaml")); got != "precious\n" {
		t.Errorf("the file that appeared mid-fetch was clobbered: %q", got)
	}
	assertNoLeftovers(t, parent)
}

// ── post-install load check ───────────────────────────────────────────────────

// A published-but-broken entry used to install cleanly and fail later against
// the user's own files, with nothing pointing at the atlas.
func TestInstall_brokenCorpusFailsAndRollsBack(t *testing.T) {
	srv := newFakeAtlas(t, map[string]string{
		"/main/index.json": `{"schema_version": 1, "entries": [{"slug": "broken", "files": [".sightmap/views/checkout.yaml"]}]}`,
		"/main/entries/broken/.sightmap/views/checkout.yaml": "views:\n\t- name: bad tabs\n",
	})
	parent := t.TempDir()
	target := filepath.Join(parent, ".sightmap")

	_, err := install(t, "broken", Options{IndexURL: srv.indexURL(), Target: target})
	if err == nil {
		t.Fatal("expected a load failure")
	}
	for _, want := range []string{"broken", "does not load", "nothing was installed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
	// The message must point at the atlas entry, using corpus-relative paths —
	// not at a temporary directory the user has never heard of.
	if strings.Contains(err.Error(), string(filepath.Separator)+".sightmap-add-") {
		t.Errorf("error leaks the staging path: %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("a broken entry was installed anyway")
	}
	assertNoLeftovers(t, parent)
}

// A corpus that loads but does not validate is still installable; the user
// just needs to know the defect is the atlas entry's, not their project's.
func TestInstall_invalidCorpusInstallsWithAWarning(t *testing.T) {
	srv := newFakeAtlas(t, map[string]string{
		"/main/index.json": `{"schema_version": 1, "entries": [{"slug": "routeless", "commit": "0123456789abcdef0123456789abcdef01234567", "files": [".sightmap/views/checkout.yaml"]}]}`,
		"/0123456789abcdef0123456789abcdef01234567/entries/routeless/.sightmap/views/checkout.yaml": "version: 1\nviews:\n  - name: Checkout\n",
	})
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "routeless", Options{IndexURL: srv.indexURL(), Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(target, "views", "checkout.yaml")); statErr != nil {
		t.Fatalf("corpus not installed: %v", statErr)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a validation warning")
	}
	joined := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"routeless", "atlas entry", "validation error"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %q, want them to mention %q", joined, want)
		}
	}
}

// ── concurrency ───────────────────────────────────────────────────────────────

// One round trip per file, serialized, is the dominant cost of an install.
// Fetches run concurrently, bounded, and still write in published order.
func TestInstall_fetchesConcurrentlyButBounded(t *testing.T) {
	const n = 12
	files := map[string]string{}
	var paths []string
	for i := 0; i < n; i++ {
		p := fmt.Sprintf(".sightmap/views/v%02d.yaml", i)
		paths = append(paths, fmt.Sprintf("%q", p))
		files["/main/entries/many/"+p] = fmt.Sprintf("version: 1\n# %d\n", i)
	}
	files["/main/index.json"] = fmt.Sprintf(`{"schema_version": 1, "entries": [{"slug": "many", "files": [%s]}]}`, strings.Join(paths, ","))
	srv := newFakeAtlas(t, files)
	srv.delay = 30 * time.Millisecond
	target := filepath.Join(t.TempDir(), ".sightmap")

	res, err := install(t, "many", Options{IndexURL: srv.indexURL(), Target: target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	srv.mu.Lock()
	maxInFlight := srv.maxInFlight
	srv.mu.Unlock()
	if maxInFlight < 2 {
		t.Errorf("max in-flight fetches = %d, want concurrent fetching", maxInFlight)
	}
	if maxInFlight > FetchConcurrency {
		t.Errorf("max in-flight fetches = %d, want at most %d", maxInFlight, FetchConcurrency)
	}
	// Concurrency must not disturb the published order or the contents.
	for i, rel := range res.Files {
		want := fmt.Sprintf("views/v%02d.yaml", i)
		if rel != want {
			t.Fatalf("Files[%d] = %q, want %q", i, rel, want)
		}
		if got := readFile(t, filepath.Join(target, filepath.FromSlash(rel))); got != fmt.Sprintf("version: 1\n# %d\n", i) {
			t.Errorf("%s = %q, want the body fetched for position %d", rel, got, i)
		}
	}
}
