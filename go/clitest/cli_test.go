package clitest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	// sightmapBin is the freshly-built CLI under test, set by TestMain.
	sightmapBin string
	// repoRoot is the repository root, used to resolve repo: content checks.
	repoRoot string
)

func TestMain(m *testing.M) {
	// The package dir at test time is .../go/clitest; go.mod is one level up.
	moduleRoot, err := findUp("go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "clitest: locate module root: %v\n", err)
		os.Exit(1)
	}
	repoRoot = filepath.Dir(moduleRoot)

	tmp, err := os.MkdirTemp("", "sightmap-clitest-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "clitest: %v\n", err)
		os.Exit(1)
	}
	bin := filepath.Join(tmp, "sightmap")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/sightmap")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "clitest: build sightmap: %v\n%s", err, out)
		os.RemoveAll(tmp)
		os.Exit(1)
	}
	sightmapBin = bin

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// TestCLICases runs every case under testdata/cases/ as its own subtest.
func TestCLICases(t *testing.T) {
	const casesDir = "testdata/cases"
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatalf("read cases dir: %v", err)
	}

	ran := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(casesDir, e.Name(), "case.yaml")
		data, err := os.ReadFile(specPath)
		if err != nil {
			continue // not a case dir
		}
		var c caseSpec
		if err := yaml.Unmarshal(data, &c); err != nil {
			t.Errorf("%s: parse case.yaml: %v", e.Name(), err)
			continue
		}
		caseDir, err := filepath.Abs(filepath.Join(casesDir, e.Name()))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		ran++
		t.Run(e.Name(), func(t *testing.T) { c.check(t, caseDir) })
	}
	if ran == 0 {
		t.Fatalf("no cases found under %s", casesDir)
	}
}

// findUp walks up from the current working directory until it finds a directory
// containing name, and returns that directory.
func findUp(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found in any parent of the working directory", name)
		}
		dir = parent
	}
}
