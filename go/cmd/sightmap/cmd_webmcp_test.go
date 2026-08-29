package main

import (
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
