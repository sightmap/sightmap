package webmcp

// Tests that the examples and the fixture compile, that emission is
// deterministic, and that generated bundles never include the CommonJS
// export guard (a page leaking `module` must not have its exports clobbered).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "webmcp", "src", "runtime", "runtime.js")); err != nil {
		t.Skipf("repository root with webmcp/ not found at %s", root)
	}
	return root
}

func TestExamplesCompileAndEmit(t *testing.T) {
	root := repoRoot(t)
	examplesDir := filepath.Join(root, "webmcp", "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("webmcp/examples not in this checkout")
		}
		t.Fatal(err)
	}
	tested := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		site := e.Name()
		t.Run(site, func(t *testing.T) {
			dir := filepath.Join(examplesDir, site)
			toolsFile := filepath.Join(dir, "webmcp.tools.yaml")
			if _, err := os.Stat(toolsFile); err != nil {
				t.Skip("no manifest")
			}

			manifest, errs, _, err := LoadManifest(toolsFile)
			if err != nil {
				t.Fatal(err)
			}
			if len(errs) > 0 {
				t.Fatalf("manifest errors: %s", strings.Join(errs, "; "))
			}
			corpusDir := filepath.Join(dir, ManifestSightmapDir(manifest))
			corpus, err := LoadCorpus(corpusDir)
			if err != nil {
				t.Fatal(err)
			}
			ir, cerrs, _ := Compile(corpus, manifest)
			if len(cerrs) > 0 {
				t.Fatalf("compile errors: %s", strings.Join(cerrs, "; "))
			}
			if len(ToolNames(ir)) == 0 {
				t.Fatal("compiled zero tools")
			}

			hash, err := CorpusHash(corpus.Dir, corpus.Files)
			if err != nil {
				t.Fatal(err)
			}
			manifestRel, _ := filepath.Rel(dir, toolsFile)
			corpusRel, _ := filepath.Rel(dir, corpus.Dir)
			prov := Provenance{
				GeneratorVersion: GeneratorVersion,
				Manifest:         manifestRel,
				Corpus:           corpusRel,
				CorpusFiles:      len(corpus.Files),
				CorpusHash:       hash,
			}
			for _, format := range Formats {
				got, err := Emit(ir, format, prov)
				if err != nil {
					t.Fatal(err)
				}
				again, err := Emit(ir, format, prov)
				if err != nil {
					t.Fatal(err)
				}
				if got != again {
					t.Fatalf("%s: emission is not deterministic", format)
				}
				if !strings.Contains(got, "__smwBoot(__SMW_META, __SMW_TOOLS);") {
					t.Fatalf("%s: missing boot call", format)
				}
				if strings.Contains(got, "module.exports") {
					t.Fatalf("%s: CommonJS export guard leaked into the bundle", format)
				}
				if !strings.Contains(got, "sightmap webmcp generate") {
					t.Fatalf("%s: banner missing shipped CLI", format)
				}
				if strings.Contains(got, "@sightmap/webmcp-codegen") {
					t.Fatalf("%s: banner still names the deleted Node package", format)
				}
			}
		})
		tested++
	}
	if tested == 0 {
		t.Skip("no example directories")
	}
}

func TestEmitBannerNamesTheShippedCLI(t *testing.T) {
	ir := fixtureCompile(t)
	got, err := Emit(ir, "snippet", Provenance{
		GeneratorVersion: "0.1.0",
		Manifest:         "webmcp.tools.yaml",
		Corpus:           ".sightmap",
		CorpusFiles:      2,
		CorpusHash:       strings.Repeat("ab", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "regenerate: sightmap webmcp generate") {
		t.Fatalf("banner missing shipped CLI:\n%s", got)
	}
	if strings.Contains(got, "sightmap-webmcp generate") || strings.Contains(got, "@sightmap/webmcp-codegen") {
		t.Fatal("banner still names the deleted Node generator")
	}
}

func TestFixtureCompiles(t *testing.T) {
	root := repoRoot(t)
	fixture := filepath.Join(root, "webmcp", "test", "fixtures", "site")
	manifest, errs, warns, err := LoadManifest(filepath.Join(fixture, "webmcp.tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("manifest errors: %v", errs)
	}
	corpus, err := LoadCorpus(filepath.Join(fixture, ".sightmap"))
	if err != nil {
		t.Fatal(err)
	}
	ir, cerrs, cwarns := Compile(corpus, manifest)
	if len(cerrs) > 0 {
		t.Fatalf("compile errors: %v", cerrs)
	}
	if len(warns)+len(cwarns) > 0 {
		t.Fatalf("unexpected warnings: %v %v", warns, cwarns)
	}
	names := ToolNames(ir)
	want := []string{"search", "search_api", "stock", "buy_first_match"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestScaffoldValidatesAgainstItsCorpus(t *testing.T) {
	root := repoRoot(t)
	corpusDir := filepath.Join(root, "web", "src", "data", "atlas", "ikea", ".sightmap")
	if _, err := os.Stat(corpusDir); err != nil {
		t.Skip("atlas corpus not present")
	}
	corpus, err := LoadCorpus(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Scaffold(corpus, "ikea", "https://www.ikea.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sightmap-webmcp init") {
		t.Fatal("scaffold still names the deleted Node bin")
	}
	if !strings.Contains(out, "sightmap webmcp init") {
		t.Fatal("scaffold missing shipped CLI")
	}
	doc, err := parseYAMLOrdered([]byte(out))
	if err != nil {
		t.Fatalf("scaffold output is not valid YAML: %v", err)
	}
	var d diags
	validateManifest(doc, &d)
	if len(d.errors) > 0 {
		t.Fatalf("scaffold output fails validation: %v", d.errors)
	}
	if _, cerrs, _ := Compile(corpus, doc); len(cerrs) > 0 {
		t.Fatalf("scaffold output fails compile: %v", cerrs)
	}
}
