package match_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// A widget host, as in an MCP Apps page: the host page has its own view, and
// the widget's document, in an iframe at another URL, is another view's.
const framesCorpus = `
version: 1
components:
  - name: Button
    selector: button
views:
  - name: Host
    route: /
    components:
      - name: Shell
        selector: .shell
      - name: WidgetFrame
        selector: iframe
      - name: HostButton
        selector: .shell button
      - name: HostOnly
        selector: .shell .note
      - name: Masked
        selector: .masked
        privacy: mask
  - name: Widget
    route: /sandbox
    components:
      - name: DateWindow
        selector: .window
        properties:
          - name: label
            extract: { from: dom.raw_text }
      - name: Inner
        selector: .shell button
`

func frameCorpus(t *testing.T) *sightmap.Corpus {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(framesCorpus), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := sightmap.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fel(tag, class string) *sightmap.Element {
	e := &sightmap.Element{Tag: tag}
	if class != "" {
		e.Classes = []string{class}
		e.Attrs = map[string]string{"class": class}
	}
	return e
}

// hostWithWidget builds: .shell > iframe > [frame doc at /sandbox: .window > button; .shell > button],
// and a span.note in the frame that only a host-view selector would name.
func hostWithWidget() (root, iframe, doc, window, winBtn, innerBtn, note *sightmap.ComponentNode) {
	winBtn = &sightmap.ComponentNode{Id: "1_4", Element: fel("button", "")}
	window = &sightmap.ComponentNode{Id: "1_3", RawText: "Mar 14-17", Element: fel("div", "window"), Children: []*sightmap.ComponentNode{winBtn}}
	innerBtn = &sightmap.ComponentNode{Id: "1_6", Element: fel("button", "")}
	innerShell := &sightmap.ComponentNode{Id: "1_5", Element: fel("div", "shell"), Children: []*sightmap.ComponentNode{innerBtn}}
	note = &sightmap.ComponentNode{Id: "1_7", Element: fel("span", "note")}
	doc = &sightmap.ComponentNode{Id: "1_1", FrameURL: "http://127.0.0.1:5274/sandbox?csp=x", Element: fel("html", ""),
		Children: []*sightmap.ComponentNode{window, innerShell, note}}
	iframe = &sightmap.ComponentNode{Id: "3", Element: fel("iframe", ""), Children: []*sightmap.ComponentNode{doc}}
	root = &sightmap.ComponentNode{Id: "2", Element: fel("div", "shell"), Children: []*sightmap.ComponentNode{iframe}}
	return
}

func TestMatch_FrameMatchedAtItsOwnURL(t *testing.T) {
	root, iframe, _, window, winBtn, _, _ := hostWithWidget()
	res := match.NewMatcher(frameCorpus(t)).Match(root, "http://localhost:5274/")

	name := func(n *sightmap.ComponentNode) string {
		if m := res[n]; m != nil {
			return m.Name
		}
		return ""
	}
	if got := name(root); got != "Shell" {
		t.Errorf("host root = %q, want Shell (page's view)", got)
	}
	if got := name(iframe); got != "WidgetFrame" {
		t.Errorf("iframe = %q, want WidgetFrame", got)
	}
	// The widget's components come from the view its own URL selects, not the host's.
	if got := name(window); got != "DateWindow" {
		t.Errorf("frame .window = %q, want DateWindow (the frame URL's view)", got)
	}
	if got := name(winBtn); got != "Button" {
		t.Errorf("frame button = %q, want the global Button", got)
	}
	if m := res[window]; m == nil || len(m.Properties) != 1 || m.Properties[0].Value != "Mar 14-17" {
		t.Errorf("frame component properties not resolved: %+v", m)
	}
}

func TestMatch_SelectorsDoNotCrossFrameBoundary(t *testing.T) {
	root, _, _, _, winBtn, innerBtn, note := hostWithWidget()
	res := match.NewMatcher(frameCorpus(t)).Match(root, "http://localhost:5274/")
	// The host view's HostButton is `.shell button`, and the host's .shell is an
	// ancestor of every button in the frame. It must not claim them: the frame
	// is another document, and a selector's ancestors stop at its edge.
	if m := res[winBtn]; m == nil || m.Name != "Button" {
		t.Errorf("frame button under .window = %+v, want the global Button (host selectors do not reach in)", m)
	}
	// Inside the frame a selector works as usual: Inner is `.shell button`, scoped
	// by the frame's own .shell.
	if m := res[innerBtn]; m == nil || m.Name != "Inner" {
		t.Errorf("frame button under the frame's .shell = %+v, want Inner", m)
	}
	// The host view's `.shell .note` has an ancestor, the host's .shell, above the
	// frame; the frame's view has no rule for .note, so nothing may name it.
	if m := res[note]; m != nil {
		t.Errorf("frame .note = %q: a host selector reached across the frame boundary", m.Name)
	}
}

func TestMatch_FramePageURLIsNotTheFramesView(t *testing.T) {
	root, _, _, window, _, _, _ := hostWithWidget()
	// Same tree, but the host's URL is the widget's route: the frame still matches
	// at its own URL (/sandbox), the host does not become the widget view.
	res := match.NewMatcher(frameCorpus(t)).Match(root, "http://localhost:5274/elsewhere")
	if m := res[window]; m == nil || m.Name != "DateWindow" {
		t.Fatalf("frame component = %+v, want DateWindow regardless of the page URL", m)
	}
	if m := res[root]; m != nil {
		t.Errorf("host root matched %q on a URL with no view", m.Name)
	}
}

func TestMatch_PrivacyIsInheritedIntoFrame(t *testing.T) {
	// A masked iframe's document is masked: a frame component's content
	// properties are withheld, though the frame's own view declares no privacy.
	root, iframe, _, window, _, _, _ := hostWithWidget()
	iframe.Element = fel("iframe", "masked")
	m := match.NewMatcher(frameCorpus(t))
	res := m.Match(root, "http://localhost:5274/")
	cm := res[window]
	if cm == nil {
		t.Fatal("frame component not matched")
	}
	if cm.Privacy != "mask" {
		t.Errorf("frame component privacy = %q, want mask inherited from the iframe", cm.Privacy)
	}
	if len(cm.Properties) != 0 || len(cm.Withheld) != 1 {
		t.Errorf("properties = %+v withheld = %v, want the text property withheld", cm.Properties, cm.Withheld)
	}
	if got := m.Privacy(root, "http://localhost:5274/")[window]; got != "mask" {
		t.Errorf("Privacy() of frame node = %q, want mask", got)
	}
}

func TestMatch_CachedMatchesUncachedWithFrames(t *testing.T) {
	root, iframe, _, _, _, _, _ := hostWithWidget()
	iframe.Element = fel("iframe", "masked")
	m := match.NewMatcher(frameCorpus(t))
	want := m.Match(root, "http://localhost:5274/")
	got, _ := m.MatchConflictsCached(match.NewClaimCache(), root, "http://localhost:5274/")
	if len(got) != len(want) {
		t.Fatalf("cached matched %d nodes, uncached %d", len(got), len(want))
	}
	for n, w := range want {
		g := got[n]
		if g == nil || g.Name != w.Name || g.Privacy != w.Privacy || len(g.Properties) != len(w.Properties) {
			t.Errorf("node %s: cached %+v, uncached %+v", n.Id, g, w)
		}
	}
}
