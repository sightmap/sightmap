package skillgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcile_seedsAnAbsentFile(t *testing.T) {
	fresh := []byte("# Title\n\n" + beginRegion("x") + "\ngenerated\n" + endRegion("x") + "\n\nauthor prose\n")
	result, changed, unmanaged := reconcile(fresh, nil)
	if unmanaged {
		t.Fatal("a file with no existing content must never be reported unmanaged")
	}
	if !changed {
		t.Error("seeding an absent file should report changed=true")
	}
	if string(result) != string(fresh) {
		t.Errorf("result = %q, want the fresh seed verbatim: %q", result, fresh)
	}
}

func TestReconcile_preservesAuthorRegionsAndOrder(t *testing.T) {
	existing := []byte("# Library UI\n\n" +
		"> a hand-corrected summary\n\n" +
		beginRegion("sub-features") + "\nOLD sub-features\n" + endRegion("sub-features") + "\n\n" +
		"## How to get to it (user POV)\n\nUsers call this the library.\n\n" +
		beginRegion("driving") + "\nOLD driving\n" + endRegion("driving") + "\n\n" +
		"## Gotchas\n\n- BulkActionTable also exists in dqm-ui.\n")

	fresh := []byte("# Library UI\n\n" +
		"> derived seed summary\n\n" +
		beginRegion("sub-features") + "\nNEW sub-features\n" + endRegion("sub-features") + "\n\n" +
		"## How to get to it (user POV)\n\n_Not yet described._\n\n" +
		beginRegion("driving") + "\nNEW driving\n" + endRegion("driving") + "\n\n" +
		"## Gotchas\n\n_None recorded yet._\n")

	result, changed, unmanaged := reconcile(fresh, existing)
	if unmanaged {
		t.Fatal("a file with matching region names must never be reported unmanaged")
	}
	if !changed {
		t.Fatal("the managed regions differ, so this must report changed=true")
	}
	got := string(result)

	for _, want := range []string{
		"> a hand-corrected summary",
		"NEW sub-features",
		"Users call this the library.",
		"NEW driving",
		"- BulkActionTable also exists in dqm-ui.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result missing %q:\n%s", want, got)
		}
	}
	for _, notWant := range []string{"derived seed summary", "OLD sub-features", "OLD driving", "_Not yet described._", "_None recorded yet._"} {
		if strings.Contains(got, notWant) {
			t.Errorf("result should not contain stale/seed text %q:\n%s", notWant, got)
		}
	}

	// Order must still be: H1/summary, sub-features, user-pov, driving, gotchas.
	idx := func(s string) int { return strings.Index(got, s) }
	if !(idx("a hand-corrected summary") < idx("NEW sub-features") &&
		idx("NEW sub-features") < idx("Users call this the library.") &&
		idx("Users call this the library.") < idx("NEW driving") &&
		idx("NEW driving") < idx("BulkActionTable also exists")) {
		t.Errorf("region/prose order was not preserved:\n%s", got)
	}
}

func TestReconcile_isIdempotent(t *testing.T) {
	fresh := []byte("# Title\n\n> seed\n\n" + beginRegion("x") + "\ngenerated v1\n" + endRegion("x") + "\n\nauthor prose\n")

	once, _, _ := reconcile(fresh, nil)
	twice, changed, unmanaged := reconcile(fresh, once)
	if unmanaged {
		t.Fatal("reconciling fresh against its own seed must never be unmanaged")
	}
	if changed {
		t.Error("reconcile(fresh, reconcile(fresh, nil)) should report unchanged — this is what makes --check trustworthy")
	}
	if string(once) != string(twice) {
		t.Errorf("merge(merge(x)) != merge(x):\nonce:  %q\ntwice: %q", once, twice)
	}
}

func TestReconcile_reportsUnmanagedForAFileWithNoMatchingRegion(t *testing.T) {
	fresh := []byte("# Title\n\n" + beginRegion("sub-features") + "\ngenerated\n" + endRegion("sub-features") + "\n")
	existing := []byte("# Title\n\nHand-written content with no sightmap markers at all.\n")

	result, changed, unmanaged := reconcile(fresh, existing)
	if !unmanaged {
		t.Fatal("a fresh render with a region absent from existing must be reported unmanaged")
	}
	if changed {
		t.Error("an unmanaged file must never be reported changed")
	}
	if string(result) != string(existing) {
		t.Error("an unmanaged file's content must be returned untouched")
	}
}

func TestReconcile_fullyGeneratedFileHasNoRegions(t *testing.T) {
	// SKILL.md and references/README.md declare no managed regions at all —
	// they are fully regenerated whenever they differ, with no splicing.
	fresh := []byte("# Router\n\nfresh content\n")
	existing := []byte("# Router\n\nstale content\n")

	result, changed, unmanaged := reconcile(fresh, existing)
	if unmanaged {
		t.Fatal("a marker-free fresh render must never be reported unmanaged")
	}
	if !changed {
		t.Fatal("differing marker-free content must report changed=true")
	}
	if string(result) != string(fresh) {
		t.Errorf("result = %q, want the fresh render verbatim (no regions to preserve)", result)
	}
}

func TestWrite_thenCheckTree_isClean(t *testing.T) {
	root := t.TempDir()
	files := []File{
		{Path: "SKILL.md", Content: []byte("# Router\n\nfresh\n")},
		{Path: "references/areas/library-ui.md", Content: []byte(
			"# Library UI\n\n> seed\n\n" + beginRegion("sub-features") + "\ngenerated\n" + endRegion("sub-features") + "\n\nprose\n",
		)},
	}

	if _, err := Write(root, files); err != nil {
		t.Fatalf("Write: %v", err)
	}
	res, err := CheckTree(root, files)
	if err != nil {
		t.Fatalf("CheckTree: %v", err)
	}
	if !res.OK() {
		t.Errorf("CheckTree after Write should be clean: %+v", res)
	}
}

func TestCheckTree_reportsMissingAndStale(t *testing.T) {
	root := t.TempDir()
	areaContent := []byte("# Library UI\n\n> seed\n\n" + beginRegion("sub-features") + "\ngenerated v1\n" + endRegion("sub-features") + "\n\nprose\n")
	if err := os.MkdirAll(filepath.Join(root, "references/areas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "references/areas/library-ui.md"), areaContent, 0o644); err != nil {
		t.Fatal(err)
	}

	stale := []byte("# Library UI\n\n> seed\n\n" + beginRegion("sub-features") + "\ngenerated v2\n" + endRegion("sub-features") + "\n\nprose\n")
	files := []File{
		{Path: "references/areas/library-ui.md", Content: stale},
		{Path: "references/areas/settings-ui.md", Content: []byte("# Settings\n\ncontent\n")},
	}

	res, err := CheckTree(root, files)
	if err != nil {
		t.Fatalf("CheckTree: %v", err)
	}
	if len(res.Stale) != 1 || res.Stale[0].Path != "references/areas/library-ui.md" {
		t.Errorf("Stale = %+v, want exactly library-ui.md", res.Stale)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "references/areas/settings-ui.md" {
		t.Errorf("Missing = %+v, want exactly settings-ui.md", res.Missing)
	}
	if res.OK() {
		t.Error("Result.OK() should be false when there's drift")
	}
}

func TestWrite_neverDeletesAFileTheNewPlanOmits(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "references/areas"), 0o755); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "references/areas/removed-area.md")
	if err := os.WriteFile(orphan, []byte("# Removed Area\n\nstill here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The new plan no longer produces removed-area.md.
	if _, err := Write(root, []File{{Path: "SKILL.md", Content: []byte("# Router\n\nfresh\n")}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("Write must never delete a file it didn't emit: %v", err)
	}
}
