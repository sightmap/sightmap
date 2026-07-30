// Command sightmap-evals runs the transcript-derived eval cases under
// evals/cases/ against a built sightmap binary.
//
// Each case is a directory containing a case.yaml (metadata + command +
// expectations) and usually a fixture/ directory holding the .sightmap/ state
// that reproduces a moment from a real authoring transcript. The runner copies
// the fixture to a temp dir, executes the sightmap command there with an
// isolated $HOME, and checks the combined output, exit code, produced files,
// and (for docs-only cases) repository files.
//
// Outcome semantics (see evals/README.md):
//
//	pass   — all expectations hold and the case is not marked known_failing
//	known  — expectations fail and the case IS marked known_failing (expected;
//	         the case documents a desired behavior awaiting its recommended fix)
//	FIXED  — a known_failing case now passes: flip known_failing to false
//	FAIL   — a case not marked known_failing fails: a regression
//	skip   — the case's requirements are not met on this machine
//
// The process exits nonzero on any FAIL or FIXED so CI keeps the dataset
// honest in both directions.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type caseSpec struct {
	ID             string     `yaml:"id"`
	Title          string     `yaml:"title"`
	FailureMode    string     `yaml:"failure_mode"`
	Source         sourceSpec `yaml:"source"`
	Requires       []string   `yaml:"requires"`
	Run            runSpec    `yaml:"run"`
	Expect         expectSpec `yaml:"expect"`
	KnownFailing   bool       `yaml:"known_failing"`
	RecommendedFix string     `yaml:"recommended_fix"`
	Notes          string     `yaml:"notes"`
}

type sourceSpec struct {
	Transcript string `yaml:"transcript"`
	Moment     string `yaml:"moment"`
}

type runSpec struct {
	Args           []string `yaml:"args"`
	Fixture        string   `yaml:"fixture"`
	PathPrepend    string   `yaml:"path_prepend"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
}

type expectSpec struct {
	Exit   string        `yaml:"exit"` // "zero", "nonzero", or "" / "any"
	Output []outputCheck `yaml:"output"`
	Files  []fileCheck   `yaml:"files"`
	Docs   []docCheck    `yaml:"docs"`
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

type docCheck struct {
	File         string `yaml:"file"` // path relative to the repo root
	MustMatch    string `yaml:"must_match"`
	MustNotMatch string `yaml:"must_not_match"`
	Why          string `yaml:"why"`
}

type result struct {
	c        caseSpec
	outcome  string // pass | known | FIXED | FAIL | skip
	failures []string
	output   string
	skipWhy  string
}

func main() {
	bin := flag.String("bin", "", "Path to the sightmap binary under test (or $SIGHTMAP_BIN)")
	casesDir := flag.String("cases", "evals/cases", "Directory containing eval case dirs")
	repoRoot := flag.String("repo", "", "Repository root for docs checks (default: parent of the cases dir's parent)")
	only := flag.String("only", "", "Run only the case with this id (or a comma-separated list)")
	verbose := flag.Bool("verbose", false, "Print each case's command output")
	flag.Parse()

	if *bin == "" {
		*bin = os.Getenv("SIGHTMAP_BIN")
	}

	absCases, err := filepath.Abs(*casesDir)
	if err != nil {
		fatal("resolve cases dir: %v", err)
	}
	root := *repoRoot
	if root == "" {
		root = filepath.Dir(filepath.Dir(absCases)) // evals/cases -> repo root
	}

	entries, err := os.ReadDir(absCases)
	if err != nil {
		fatal("read cases dir %s: %v", absCases, err)
	}

	onlySet := map[string]bool{}
	if *only != "" {
		for _, id := range strings.Split(*only, ",") {
			onlySet[strings.TrimSpace(id)] = true
		}
	}

	var results []result
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		caseDir := filepath.Join(absCases, e.Name())
		specPath := filepath.Join(caseDir, "case.yaml")
		data, err := os.ReadFile(specPath)
		if err != nil {
			continue // not a case dir
		}
		var c caseSpec
		if err := yaml.Unmarshal(data, &c); err != nil {
			fatal("%s: parse case.yaml: %v", e.Name(), err)
		}
		if c.ID == "" {
			c.ID = e.Name()
		}
		if len(onlySet) > 0 && !onlySet[c.ID] {
			continue
		}
		results = append(results, runCase(c, caseDir, root, *bin, *verbose))
	}

	if len(results) == 0 {
		fatal("no eval cases found under %s", absCases)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].c.ID < results[j].c.ID })
	report(results, *verbose)

	exit := 0
	for _, r := range results {
		if r.outcome == "FAIL" || r.outcome == "FIXED" {
			exit = 1
		}
	}
	os.Exit(exit)
}

func runCase(c caseSpec, caseDir, repoRoot, bin string, verbose bool) result {
	r := result{c: c}

	if why, ok := requirementsMet(c.Requires); !ok {
		r.outcome = "skip"
		r.skipWhy = why
		return r
	}

	var failures []string

	// ── Exec check (optional: docs-only cases have no run.args) ──
	if len(c.Run.Args) > 0 {
		if bin == "" {
			r.outcome = "skip"
			r.skipWhy = "no sightmap binary (--bin or $SIGHTMAP_BIN)"
			return r
		}
		out, exitCode, err := execCase(c, caseDir, bin)
		r.output = out
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
		}
	}

	// ── Docs checks (against repo files) ──
	for _, dc := range c.Expect.Docs {
		data, err := os.ReadFile(filepath.Join(repoRoot, dc.File))
		if err != nil {
			failures = append(failures, fmt.Sprintf("docs check: read %s: %v", dc.File, err))
			continue
		}
		failures = append(failures, checkPattern(dc.File, string(data), dc.MustMatch, dc.MustNotMatch, dc.Why)...)
	}

	r.failures = failures
	switch {
	case len(failures) == 0 && !c.KnownFailing:
		r.outcome = "pass"
	case len(failures) == 0 && c.KnownFailing:
		r.outcome = "FIXED"
	case len(failures) > 0 && c.KnownFailing:
		r.outcome = "known"
	default:
		r.outcome = "FAIL"
	}
	return r
}

// execCase copies the fixture to a temp dir, runs the binary there with an
// isolated HOME, and returns combined output + exit code. File-existence
// expectations are evaluated against the temp cwd before it is discarded.
func execCase(c caseSpec, caseDir, bin string) (string, int, error) {
	tmp, err := os.MkdirTemp("", "sightmap-eval-"+c.ID+"-")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmp)

	cwd := filepath.Join(tmp, "fixture")
	if c.Run.Fixture != "" {
		if err := copyDir(filepath.Join(caseDir, c.Run.Fixture), cwd); err != nil {
			return "", 0, fmt.Errorf("copy fixture: %w", err)
		}
	} else if err := os.MkdirAll(cwd, 0o755); err != nil {
		return "", 0, err
	}

	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", 0, err
	}

	timeout := time.Duration(c.Run.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	absBin, err := filepath.Abs(bin)
	if err != nil {
		return "", 0, err
	}
	cmd := exec.CommandContext(ctx, absBin, c.Run.Args...)
	cmd.Dir = cwd

	env := []string{"HOME=" + home}
	path := os.Getenv("PATH")
	if c.Run.PathPrepend != "" {
		path = filepath.Join(caseDir, c.Run.PathPrepend) + string(os.PathListSeparator) + path
	}
	env = append(env, "PATH="+path)
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		if k == "HOME" || k == "PATH" {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env

	out, runErr := cmd.CombinedOutput()
	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
			runErr = nil
		} else if ctx.Err() == context.DeadlineExceeded {
			return string(out), 0, fmt.Errorf("timed out after %s", timeout)
		}
	}
	if runErr != nil {
		return string(out), 0, runErr
	}

	// File-existence checks run against the temp cwd before cleanup.
	var fileFailures []string
	for _, fc := range c.Expect.Files {
		if _, statErr := os.Stat(filepath.Join(cwd, fc.Exists)); statErr != nil {
			why := fc.Why
			if why != "" {
				why = " (" + why + ")"
			}
			fileFailures = append(fileFailures, fmt.Sprintf("expected %s to exist after run%s", fc.Exists, why))
		}
	}
	if len(fileFailures) > 0 {
		return string(out) + "\n[file checks]\n" + strings.Join(fileFailures, "\n"), exitCode,
			fmt.Errorf("%s", strings.Join(fileFailures, "; "))
	}
	return string(out), exitCode, nil
}

// checkPattern applies must_match / must_not_match regexes to text and returns
// human-readable failure strings.
func checkPattern(label, text, mustMatch, mustNotMatch, why string) []string {
	var failures []string
	suffix := ""
	if why != "" {
		suffix = " — " + why
	}
	if mustMatch != "" {
		re, err := regexp.Compile(mustMatch)
		if err != nil {
			return []string{fmt.Sprintf("%s: bad must_match regex %q: %v", label, mustMatch, err)}
		}
		if !re.MatchString(text) {
			failures = append(failures, fmt.Sprintf("%s does not match %q%s", label, mustMatch, suffix))
		}
	}
	if mustNotMatch != "" {
		re, err := regexp.Compile(mustNotMatch)
		if err != nil {
			return []string{fmt.Sprintf("%s: bad must_not_match regex %q: %v", label, mustNotMatch, err)}
		}
		if re.MatchString(text) {
			failures = append(failures, fmt.Sprintf("%s matches forbidden %q%s", label, mustNotMatch, suffix))
		}
	}
	return failures
}

// requirementsMet reports whether every entry in requires is satisfied on this
// machine, and a human-readable reason when one is not.
func requirementsMet(requires []string) (string, bool) {
	for _, req := range requires {
		switch req {
		case "":
			continue
		case "linux", "darwin", "windows":
			if runtime.GOOS != req {
				return "requires: " + req, false
			}
		case "chrome":
			if !chromeAvailable() {
				return "requires: chrome (none found in PATH; set SIGHTMAP_EVALS_CHROME=1 to force)", false
			}
		case "manual":
			return "requires: manual (needs a live browser session; run by hand)", false
		default:
			return "unknown requirement: " + req, false
		}
	}
	return "", true
}

func chromeAvailable() bool {
	if os.Getenv("SIGHTMAP_EVALS_CHROME") == "1" {
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
// helper scripts stay executable. Dotfiles (e.g. .sightmap/.session) are
// included.
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
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
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

func report(results []result, verbose bool) {
	counts := map[string]int{}
	fmt.Printf("%-7s %-42s %-5s %s\n", "RESULT", "CASE", "FM", "NOTE")
	fmt.Println(strings.Repeat("─", 100))
	for _, r := range results {
		counts[r.outcome]++
		note := ""
		switch r.outcome {
		case "known":
			note = "expected failure — awaiting " + orDash(r.c.RecommendedFix)
		case "FIXED":
			note = "now passes — flip known_failing to false in case.yaml"
		case "FAIL":
			note = "REGRESSION: " + strings.Join(r.failures, "; ")
		case "skip":
			note = r.skipWhy
		}
		fmt.Printf("%-7s %-42s %-5s %s\n", r.outcome, r.c.ID, orDash(r.c.FailureMode), note)
		if (verbose || r.outcome == "FAIL") && len(r.failures) > 0 {
			for _, f := range r.failures {
				fmt.Printf("        · %s\n", f)
			}
		}
		if verbose && r.output != "" {
			fmt.Println("        ── output ──")
			for _, line := range strings.Split(strings.TrimRight(r.output, "\n"), "\n") {
				fmt.Printf("        %s\n", line)
			}
		}
	}
	fmt.Println(strings.Repeat("─", 100))
	fmt.Printf("pass=%d known=%d fixed=%d fail=%d skip=%d (total %d)\n",
		counts["pass"], counts["known"], counts["FIXED"], counts["FAIL"], counts["skip"], len(results))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sightmap-evals: "+format+"\n", args...)
	os.Exit(1)
}
