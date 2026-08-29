package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureSiteDir returns <repo>/webmcp/test/fixtures/site, skipping when the
// test runs outside a full repo checkout.
func fixtureSiteDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "webmcp", "test", "fixtures", "site"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "webmcp.tools.yaml")); err != nil {
		t.Skip("webmcp fixture not present (not a full repo checkout)")
	}
	return dir
}

func TestDisplayPathPrefersCwdRelative(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	abs := filepath.Join(dir, "fixture.webmcp.js")
	if got := displayPath(abs); got != "fixture.webmcp.js" {
		t.Fatalf("displayPath(%q) = %q, want fixture.webmcp.js", abs, got)
	}
	outside := filepath.Join(filepath.Dir(dir), "elsewhere.js")
	if got := displayPath(outside); got != outside {
		t.Fatalf("outside path: got %q, want %q", got, outside)
	}
}

func TestWebmcpUnknownSubcommand(t *testing.T) {
	if err := runWebmcp([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("err = %v, want unknown subcommand", err)
	}
}

func TestWebmcpValidateFixture(t *testing.T) {
	site := fixtureSiteDir(t)
	err := runWebmcp([]string{"validate",
		"--tools", filepath.Join(site, "webmcp.tools.yaml"),
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestWebmcpValidateMissingManifest(t *testing.T) {
	err := runWebmcp([]string{"validate", "--tools", filepath.Join(t.TempDir(), "nope.yaml")})
	if err == nil || !strings.Contains(err.Error(), "tools manifest not found") {
		t.Fatalf("err = %v, want manifest-not-found", err)
	}
}

func TestWebmcpGenerateRejectsUnknownFormat(t *testing.T) {
	site := fixtureSiteDir(t)
	err := runWebmcp([]string{"generate",
		"--tools", filepath.Join(site, "webmcp.tools.yaml"),
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--format", "wasm",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("err = %v, want unknown format", err)
	}
}

func TestWebmcpGenerateWritesAllFormats(t *testing.T) {
	site := fixtureSiteDir(t)
	out := t.TempDir()
	err := runWebmcp([]string{"generate",
		"--tools", filepath.Join(site, "webmcp.tools.yaml"),
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--format", "all",
		"--out-dir", out,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, name := range []string{"fixture.webmcp.js", "fixture.webmcp.module.js", "fixture.webmcp.user.js"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if !strings.Contains(string(data), "__smwBoot(__SMW_META, __SMW_TOOLS);") {
			t.Fatalf("%s does not look like a generated bundle", name)
		}
		if !strings.Contains(string(data), "sightmap webmcp generate") {
			t.Fatalf("%s banner missing shipped CLI", name)
		}
	}
}

func TestWebmcpGenerateCheckFreshAndStale(t *testing.T) {
	site := fixtureSiteDir(t)
	out := t.TempDir()
	args := []string{"generate",
		"--tools", filepath.Join(site, "webmcp.tools.yaml"),
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--format", "snippet",
		"--out-dir", out,
	}
	if err := runWebmcp(args); err != nil {
		t.Fatalf("generate: %v", err)
	}
	check := append(args, "--check")
	if err := runWebmcp(check); err != nil {
		t.Fatalf("fresh --check: %v", err)
	}
	stale := filepath.Join(out, "fixture.webmcp.js")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runWebmcp(check)
	if !errors.Is(err, errGenerateDrift) {
		t.Fatalf("stale --check err = %v, want errGenerateDrift", err)
	}
}

func TestWebmcpGenerateRejectsOutWithAllFormats(t *testing.T) {
	site := fixtureSiteDir(t)
	err := runWebmcp([]string{"generate",
		"--tools", filepath.Join(site, "webmcp.tools.yaml"),
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--format", "all",
		"--out", filepath.Join(t.TempDir(), "one.js"),
	})
	if err == nil || !strings.Contains(err.Error(), "--out only works with a single --format") {
		t.Fatalf("err = %v, want --out vs all", err)
	}
}

func TestWebmcpInitRoundTrips(t *testing.T) {
	site := fixtureSiteDir(t)
	out := filepath.Join(t.TempDir(), "draft.yaml")
	if err := runWebmcp([]string{"init",
		"--site", "fixture", "--base-url", "https://shop.example",
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--out", out,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runWebmcp([]string{"validate",
		"--tools", out,
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
	}); err != nil {
		t.Fatalf("validate of scaffolded draft: %v", err)
	}
	// init refuses to overwrite.
	err := runWebmcp([]string{"init",
		"--site", "fixture", "--base-url", "https://shop.example",
		"--sightmap-dir", filepath.Join(site, ".sightmap"),
		"--out", out,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already-exists refusal", err)
	}
}
