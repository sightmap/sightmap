package webmcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func fixtureCorpus(t *testing.T) *Corpus {
	t.Helper()
	root := repoRoot(t)
	corpus, err := LoadCorpus(filepath.Join(root, "webmcp", "test", "fixtures", "site", ".sightmap"))
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

func fixtureCompile(t *testing.T) *OM {
	t.Helper()
	root := repoRoot(t)
	manifest, errs, _, err := LoadManifest(filepath.Join(root, "webmcp", "test", "fixtures", "site", "webmcp.tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Fatalf("manifest errors: %s", strings.Join(errs, "; "))
	}
	ir, cerrs, _ := Compile(fixtureCorpus(t), manifest)
	if len(cerrs) > 0 {
		t.Fatalf("compile errors: %s", strings.Join(cerrs, "; "))
	}
	return ir
}

func parseManifestYAML(t *testing.T, src string) any {
	t.Helper()
	doc, err := parseYAMLOrdered([]byte(src))
	if err != nil {
		t.Fatalf("yaml: %v\n%s", err, src)
	}
	return doc
}

func compileYAML(t *testing.T, corpus *Corpus, src string) (*OM, []string, []string) {
	t.Helper()
	return Compile(corpus, parseManifestYAML(t, src))
}

func toolNamed(ir *OM, name string) *OM {
	for _, t := range asList(omGet(ir, "tools")) {
		tom := asOM(t)
		if asString(omGet(tom, "name")) == name {
			return tom
		}
	}
	return nil
}

func stepDo(tool *OM, do string) *OM {
	flow := asOM(omGet(tool, "flow"))
	if flow == nil {
		return nil
	}
	for _, s := range asList(omGet(flow, "steps")) {
		som := asOM(s)
		if asString(omGet(som, "do")) == do {
			return som
		}
	}
	return nil
}

func joinErrs(errs []string) string { return strings.Join(errs, "\n") }

func wantContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("want %q in:\n%s", needle, haystack)
	}
}

func firstLink(target *OM) *OM {
	links := asList(omGet(target, "links"))
	if len(links) == 0 {
		return nil
	}
	return asOM(links[0])
}
