// Package clitest is a testscript-inspired harness for exercising the sightmap
// CLI's agent-facing behaviour. Each directory under testdata/cases/ is one
// case: a case.yaml describing a command plus the output/exit/file it should
// produce, and an optional fixture/ whose contents become the run's working
// directory.
//
// These are ordinary integration tests — `go test ./clitest/` runs them, and
// so does CI. The one non-standard feature is the xfail ratchet: a case marked
// known_bug: true asserts DESIRED (not-yet-shipped) behaviour, so its
// expectations are expected to fail. When such a case starts passing, the test
// fails to remind you to drop the flag. That keeps the dataset honest in both
// directions: a regression turns a passing case red, and a fix turns a
// known_bug case red until it's claimed.
//
// See README.md for the case format and the intake ritual for new cases.
package clitest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// caseSpec is one declarative CLI case. Identity is the directory name; the
// rest is metadata plus expectations. Provenance (which transcript/report a
// case came from) lives in the public issue it links to, not here.
type caseSpec struct {
	Title    string     `yaml:"title"`
	Issue    int        `yaml:"issue"`     // public GitHub issue number; 0 if not yet filed
	KnownBug bool       `yaml:"known_bug"` // expectations describe desired-but-unshipped behaviour (xfail)
	Requires []string   `yaml:"requires"`  // any of: darwin linux windows chrome manual
	Run      *runSpec   `yaml:"run"`       // omit for repo-content-only cases
	Expect   expectSpec `yaml:"expect"`
}

type runSpec struct {
	Args        []string `yaml:"args"`
	Fixture     string   `yaml:"fixture"`      // case-relative dir whose contents become the cwd
	PathPrepend string   `yaml:"path_prepend"` // cwd-relative dir prepended to PATH (fake binaries)
	TimeoutSec  int      `yaml:"timeout_seconds"`
}

type expectSpec struct {
	Exit   string        `yaml:"exit"` // zero | nonzero | "" (any)
	Output []outputCheck `yaml:"output"`
	Files  []fileCheck   `yaml:"files"`
	Repo   []repoCheck   `yaml:"repo"`
}

type outputCheck struct {
	MustMatch    string `yaml:"must_match"`
	MustNotMatch string `yaml:"must_not_match"`
	Why          string `yaml:"why"`
}

type fileCheck struct {
	Exists string `yaml:"exists"` // path relative to the run cwd
	Why    string `yaml:"why"`
}

type repoCheck struct {
	File         string `yaml:"file"` // path relative to the repo root
	MustMatch    string `yaml:"must_match"`
	MustNotMatch string `yaml:"must_not_match"`
	Why          string `yaml:"why"`
}

// check runs one case as the current subtest and reports the outcome through t.
func (c caseSpec) check(t *testing.T, caseDir string) {
	if reason, ok := requirementsMet(c.Requires); !ok {
		t.Skip(reason)
	}

	var failures []string

	if c.Run != nil {
		if sightmapBin == "" {
			t.Skip("no sightmap binary was built")
		}
		out, exitCode, cwd, err := execCase(t, c, caseDir)
		if err != nil {
			failures = append(failures, fmt.Sprintf("run error: %v", err))
		} else {
			switch c.Expect.Exit {
			case "zero":
				if exitCode != 0 {
					failures = append(failures, fmt.Sprintf("expected exit 0, got %d", exitCode))
				}
			case "nonzero":
				if exitCode == 0 {
					failures = append(failures, "expected nonzero exit, got 0")
				}
			}
			for _, oc := range c.Expect.Output {
				failures = append(failures, checkPattern("output", out, oc.MustMatch, oc.MustNotMatch, oc.Why)...)
			}
			for _, fc := range c.Expect.Files {
				if _, statErr := os.Stat(filepath.Join(cwd, fc.Exists)); statErr != nil {
					failures = append(failures, withWhy(fmt.Sprintf("expected %s to exist after run", fc.Exists), fc.Why))
				}
			}
		}
		// Surface the command output whenever something is off, so a failing
		// (or unexpectedly-fixed) case is debuggable straight from the log.
		if out != "" && (len(failures) == 0) == c.KnownBug {
			t.Logf("$ sightmap %s\n--- output ---\n%s", strings.Join(c.Run.Args, " "), strings.TrimRight(out, "\n"))
		}
	}

	for _, rc := range c.Expect.Repo {
		data, err := os.ReadFile(filepath.Join(repoRoot, rc.File))
		if err != nil {
			failures = append(failures, fmt.Sprintf("repo check: read %s: %v", rc.File, err))
			continue
		}
		failures = append(failures, checkPattern(rc.File, string(data), rc.MustMatch, rc.MustNotMatch, rc.Why)...)
	}

	switch {
	case c.KnownBug && len(failures) == 0:
		t.Errorf("known_bug case now passes — the bug looks fixed.\n"+
			"Drop `known_bug: true` from case.yaml (and update expectations if needed)%s.", issueSuffix(c))
	case c.KnownBug:
		t.Logf("known bug%s — expectations describe behaviour not yet shipped:\n  - %s",
			issueSuffix(c), strings.Join(failures, "\n  - "))
	case len(failures) > 0:
		t.Errorf("unmet expectations:\n  - %s", strings.Join(failures, "\n  - "))
	}
}

// execCase copies the fixture into an isolated temp cwd, runs the binary there
// with an isolated $HOME, and returns combined output, exit code, and the cwd
// (kept alive by t.TempDir until the subtest ends, for file-existence checks).
func execCase(t *testing.T, c caseSpec, caseDir string) (out string, exitCode int, cwd string, err error) {
	base := t.TempDir()
	cwd = filepath.Join(base, "run")
	if c.Run.Fixture != "" {
		if err := copyDir(filepath.Join(caseDir, c.Run.Fixture), cwd); err != nil {
			return "", 0, cwd, fmt.Errorf("copy fixture: %w", err)
		}
	} else if err := os.MkdirAll(cwd, 0o755); err != nil {
		return "", 0, cwd, err
	}

	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", 0, cwd, err
	}

	timeout := time.Duration(c.Run.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sightmapBin, c.Run.Args...)
	cmd.Dir = cwd

	path := os.Getenv("PATH")
	if c.Run.PathPrepend != "" {
		path = filepath.Join(cwd, c.Run.PathPrepend) + string(os.PathListSeparator) + path
	}
	env := []string{"HOME=" + home, "PATH=" + path}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k == "HOME" || k == "PATH" {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env

	outb, runErr := cmd.CombinedOutput()
	out = string(outb)
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exitCode = ee.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return out, 0, cwd, fmt.Errorf("timed out after %s", timeout)
		} else {
			return out, 0, cwd, runErr
		}
	}
	return out, exitCode, cwd, nil
}

// checkPattern applies must_match / must_not_match regexes and returns
// human-readable failure strings.
func checkPattern(label, text, mustMatch, mustNotMatch, why string) []string {
	var failures []string
	if mustMatch != "" {
		re, err := regexp.Compile(mustMatch)
		if err != nil {
			return []string{fmt.Sprintf("%s: bad must_match regex %q: %v", label, mustMatch, err)}
		}
		if !re.MatchString(text) {
			failures = append(failures, withWhy(fmt.Sprintf("%s does not match %q", label, mustMatch), why))
		}
	}
	if mustNotMatch != "" {
		re, err := regexp.Compile(mustNotMatch)
		if err != nil {
			return []string{fmt.Sprintf("%s: bad must_not_match regex %q: %v", label, mustNotMatch, err)}
		}
		if re.MatchString(text) {
			failures = append(failures, withWhy(fmt.Sprintf("%s matches forbidden %q", label, mustNotMatch), why))
		}
	}
	return failures
}

// requirementsMet reports whether every entry in requires holds on this
// machine, and a human-readable reason for the first that does not.
func requirementsMet(requires []string) (string, bool) {
	for _, req := range requires {
		switch req {
		case "":
			continue
		case "linux", "darwin", "windows":
			if runtime.GOOS != req {
				return "requires " + req, false
			}
		case "chrome":
			if !chromeAvailable() {
				return "requires chrome (none in PATH; set SIGHTMAP_TEST_CHROME=1 to force)", false
			}
		case "manual":
			return "manual: not CI-runnable (live browser or unsafe side effects) — see the case's why for hand-repro steps", false
		default:
			return "unknown requirement: " + req, false
		}
	}
	return "", true
}

func chromeAvailable() bool {
	if os.Getenv("SIGHTMAP_TEST_CHROME") == "1" {
		return true
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// copyDir copies src into dst (created), preserving file modes so fixture
// helper scripts stay executable. Dotfiles (e.g. .sightmap/.session) included.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

func withWhy(msg, why string) string {
	if why == "" {
		return msg
	}
	return msg + " — " + strings.TrimSpace(why)
}

func issueSuffix(c caseSpec) string {
	if c.Issue == 0 {
		return ""
	}
	return fmt.Sprintf(" (see #%d)", c.Issue)
}
