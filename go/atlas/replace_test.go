package atlas

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir moves the process into dir for the duration of the test. (testing.T's
// own Chdir needs a newer go directive than this module declares.)
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	})
}

// writeTree materializes a directory layout: a trailing slash means a
// directory, anything else is a file with the given content.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(p, "/")))
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A corpus directory is exactly what --force is for. Every one of these must
// stay replaceable, or the guardrail has broken the feature it guards.
func TestCheckReplaceable_allowsACorpusDirectory(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"config.yaml":            "version: 1\n",
		"components.yaml":        "version: 1\n",
		"survey.yml":             "version: 1\n",
		"views/checkout.yaml":    "version: 1\n",
		"snapshots/plp/a.snap":   "{}",
		"review/punchlist.yaml":  "- item\n",
		".DS_Store":              "\x00\x00finder\n",
		"views/nested/deep.yaml": "version: 1\n",
	})
	if err := checkReplaceable(dir); err != nil {
		t.Fatalf("a corpus directory was refused: %v", err)
	}
}

// The contents rule is an allowlist: anything that is not a .yaml/.yml file or
// one of the corpus's own subdirectories means the target is somebody's
// project. .git is the canonical tripwire — that is the case that turns
// --force into `rm -rf` on a repository.
func TestCheckReplaceable_refusesNonCorpusContents(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		blame string
	}{
		{"git-repo", map[string]string{"config.yaml": "version: 1\n", ".git/HEAD": "ref: refs/heads/main\n"}, ".git"},
		{"dotenv", map[string]string{"config.yaml": "version: 1\n", ".env": "TOKEN=hunter2\n"}, ".env"},
		{"go-module", map[string]string{"go.mod": "module example.com/app\n"}, "go.mod"},
		{"source-tree", map[string]string{"config.yaml": "version: 1\n", "src/": ""}, "src"},
		{"readme", map[string]string{"config.yaml": "version: 1\n", "README.md": "# app\n"}, "README.md"},
		{"unknown-dir", map[string]string{"config.yaml": "version: 1\n", "node_modules/": ""}, "node_modules"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, tc.files)

			err := checkReplaceable(dir)
			if !errors.Is(err, ErrUnsafeReplace) {
				t.Fatalf("error = %v, want ErrUnsafeReplace", err)
			}
			if !strings.Contains(err.Error(), tc.blame) {
				t.Errorf("error = %v, want it to name %q", err, tc.blame)
			}
			if !strings.Contains(err.Error(), dir) {
				t.Errorf("error = %v, want it to name the target %q", err, dir)
			}
		})
	}
}

// A symlink is never corpus content whatever it points at: the rename that
// replaces the target takes the link with it, and what it pointed at is not
// something an atlas entry can put back.
func TestCheckReplaceable_refusesASymlinkedCorpusDir(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"config.yaml": "version: 1\n"})
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(dir, "views")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := checkReplaceable(dir)
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("error = %v, want ErrUnsafeReplace", err)
	}
	if !strings.Contains(err.Error(), "views") {
		t.Errorf("error = %v, want it to name the symlink", err)
	}
}

// These directories are refused whatever they hold. `--target .` is one typo
// from `--target ..`, and no content inspection makes deleting either of them
// the user's intent. Each case's directory is corpus-shaped, so the location
// rule is the only thing that can be refusing it.
func TestCheckReplaceable_refusesDangerousLocations(t *testing.T) {
	t.Run("working-directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"config.yaml": "version: 1\n"})
		chdir(t, dir)

		assertUnsafeLocation(t, dir, "current working directory")
	})

	// `--target ..` from inside the corpus.
	t.Run("ancestor-of-the-working-directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"config.yaml": "version: 1\n", "views/": ""})
		chdir(t, filepath.Join(dir, "views"))

		assertUnsafeLocation(t, dir, "contains the current working directory")
	})

	t.Run("filesystem-root", func(t *testing.T) {
		root := filepath.VolumeName(mustGetwd(t)) + string(filepath.Separator)
		assertUnsafeLocation(t, root, "filesystem root")
	})

	t.Run("home-directory", func(t *testing.T) {
		home := t.TempDir()
		writeTree(t, home, map[string]string{"config.yaml": "version: 1\n"})
		setHome(t, home)

		assertUnsafeLocation(t, home, "your home directory")
	})

	t.Run("ancestor-of-the-home-directory", func(t *testing.T) {
		parent := t.TempDir()
		home := filepath.Join(parent, "views") // corpus-shaped, so only the location can refuse it
		writeTree(t, parent, map[string]string{"config.yaml": "version: 1\n"})
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatal(err)
		}
		setHome(t, home)

		assertUnsafeLocation(t, parent, "contains your home directory")
	})
}

func assertUnsafeLocation(t *testing.T, target, wantReason string) {
	t.Helper()
	err := checkReplaceable(target)
	if !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("checkReplaceable(%s) = %v, want ErrUnsafeReplace", target, err)
	}
	if !strings.Contains(err.Error(), wantReason) {
		t.Errorf("error = %v, want it to say %q", err, wantReason)
	}
	// The refusal is useless to an agent if it does not say where to install
	// instead.
	if !strings.Contains(err.Error(), ".sightmap") {
		t.Errorf("error = %v, want it to name the directory to install into instead", err)
	}
}

// setHome points os.UserHomeDir at dir on every platform.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// The comparison must not turn on which spelling of a path the caller typed:
// on macOS a temp directory is reached through /var while the working
// directory reads back as /private/var.
func TestCheckReplaceable_resolvesSymlinkedSpellingsOfTheSameDirectory(t *testing.T) {
	real := t.TempDir()
	writeTree(t, real, map[string]string{"config.yaml": "version: 1\n"})
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	chdir(t, real)

	if err := checkReplaceable(link); !errors.Is(err, ErrUnsafeReplace) {
		t.Fatalf("a symlink to the working directory was accepted: %v", err)
	}
}
