package browser

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestCftPlatformFor guards the OS/arch → CfT platform mapping. The regression
// this locks in: linux/arm64 previously resolved to "linux64" (the x64 build),
// so `browser install` fetched an x86-64 Chrome onto arm64 machines.
func TestCftPlatformFor(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "mac-arm64"},
		{"darwin", "amd64", "mac-x64"},
		{"linux", "amd64", "linux64"},
		{"linux", "arm64", "linux-arm64"}, // the fix
		{"windows", "amd64", "win64"},
		{"windows", "386", "win32"},
		{"plan9", "amd64", ""}, // unsupported
	}
	for _, c := range cases {
		if got := cftPlatformFor(c.goos, c.goarch); got != c.want {
			t.Errorf("cftPlatformFor(%q, %q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestCftRelBinLinuxArm64 checks the extracted-binary path for the arm64 build,
// which lives in its own chrome-linux-arm64/ directory (not chrome-linux64/).
func TestCftRelBinLinuxArm64(t *testing.T) {
	got := cftRelBin("linux-arm64")
	want := filepath.Join("chrome-linux-arm64", "chrome")
	if got != want {
		t.Errorf("cftRelBin(%q) = %q, want %q", "linux-arm64", got, want)
	}
	if cftRelBin("linux64") == got {
		t.Error("linux-arm64 and linux64 must not share an install path")
	}
}

// testVersionsJSON mirrors the real CfT endpoint shape: Stable omits
// linux-arm64 (true as of this writing), while Beta and Dev carry it.
const testVersionsJSON = `{
  "channels": {
    "Stable": {"version": "152.0", "downloads": {"chrome": [
      {"platform": "linux64", "url": "https://cft/stable/linux64.zip"},
      {"platform": "mac-arm64", "url": "https://cft/stable/mac-arm64.zip"}
    ]}},
    "Beta": {"version": "153.0", "downloads": {"chrome": [
      {"platform": "linux-arm64", "url": "https://cft/beta/linux-arm64.zip"},
      {"platform": "linux64", "url": "https://cft/beta/linux64.zip"}
    ]}},
    "Dev": {"version": "154.0", "downloads": {"chrome": [
      {"platform": "linux-arm64", "url": "https://cft/dev/linux-arm64.zip"}
    ]}}
  }
}`

func parseTestVersions(t *testing.T) *cftVersionsJSON {
	t.Helper()
	var data cftVersionsJSON
	if err := json.Unmarshal([]byte(testVersionsJSON), &data); err != nil {
		t.Fatalf("unmarshal test versions: %v", err)
	}
	return &data
}

func TestResolvePrefersStable(t *testing.T) {
	data := parseTestVersions(t)

	// A platform Stable carries resolves to Stable even though later channels
	// also offer it.
	v, u, ch, err := data.resolve("linux64")
	if err != nil {
		t.Fatalf("resolve(linux64): %v", err)
	}
	if ch != "Stable" || v != "152.0" || u != "https://cft/stable/linux64.zip" {
		t.Errorf("resolve(linux64) = (%q, %q, %q), want Stable/152.0", v, u, ch)
	}
}

func TestResolveFallsBackOffStable(t *testing.T) {
	data := parseTestVersions(t)

	// linux-arm64 is absent from Stable, so resolution must fall through to the
	// first channel that carries it (Beta, ahead of Dev in the preference order).
	v, u, ch, err := data.resolve("linux-arm64")
	if err != nil {
		t.Fatalf("resolve(linux-arm64): %v", err)
	}
	if ch != "Beta" {
		t.Errorf("resolve(linux-arm64) channel = %q, want Beta", ch)
	}
	if v != "153.0" || u != "https://cft/beta/linux-arm64.zip" {
		t.Errorf("resolve(linux-arm64) = (%q, %q), want Beta's 153.0 / beta url", v, u)
	}
}

func TestResolveUnknownPlatformErrors(t *testing.T) {
	data := parseTestVersions(t)

	if _, _, _, err := data.resolve("win64"); err == nil {
		t.Error("resolve(win64) should error: no channel in the fixture carries it")
	}
}
