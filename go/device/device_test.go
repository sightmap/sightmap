package device

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

func loadDescribe(t *testing.T, name string) *Describe {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var d Describe
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func loadCorpus(t *testing.T, dir string) *sightmap.Corpus {
	t.Helper()
	c, err := sightmap.Load(filepath.Join("testdata", dir))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func snapshot(t *testing.T, describe, corpus, view string) *Snapshot {
	t.Helper()
	var c *sightmap.Corpus
	if corpus != "" {
		c = loadCorpus(t, corpus)
	}
	s, err := Annotate(Normalize(loadDescribe(t, describe)), c, nil, view)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func byIdentifier(t *testing.T, tree *Tree, id string) *sightmap.ComponentNode {
	t.Helper()
	for _, n := range tree.Nodes {
		if Identifier(n) == id {
			return n
		}
	}
	t.Fatalf("no node with identifier %q", id)
	return nil
}

func TestNormalizeCollapsesWrappersAndMapsElements(t *testing.T) {
	tree := Normalize(loadDescribe(t, "describe-simple"))
	if tree.Root.Role != "application" || tree.Root.Id != "1" {
		t.Fatalf("root = %s %s", tree.Root.Id, tree.Root.Role)
	}
	for i, n := range tree.Nodes {
		if n.Role == "other" && n.Name == "" && Identifier(n) == "" && n.Value == "" && len(n.Children) <= 1 && n != tree.Root {
			t.Errorf("node %d is an unlabeled single-child wrapper that should have collapsed", i+1)
		}
	}

	toggle := byIdentifier(t, tree, "simple.toggle")
	if toggle.Role != "switch" || toggle.State["checked"] != "true" || !toggle.IsInteractive {
		t.Errorf("toggle = role %s state %v interactive %v", toggle.Role, toggle.State, toggle.IsInteractive)
	}
	field := byIdentifier(t, tree, "simple.email")
	if field.Element.Attrs["testid"] != "simple.email" || field.Element.Attrs["value"] != "Email" {
		t.Errorf("field attrs = %v", field.Element.Attrs)
	}
	if field.Element.Id != "" {
		t.Errorf("simple.email is no CSS identifier, but Element.Id = %q", field.Element.Id)
	}
	if last := byIdentifier(t, tree, "lastTapped"); last.Element.Id != "lastTapped" {
		t.Errorf("lastTapped Element.Id = %q", last.Element.Id)
	}
	tab := byIdentifier(t, tree, "1.circle")
	if tab.Role != "tab" || tab.Name != "Simple" {
		t.Errorf("tab = %s %q", tab.Role, tab.Name)
	}
}

func TestNormalizeReactNative(t *testing.T) {
	tree := Normalize(loadDescribe(t, "describe-rn"))
	row := byIdentifier(t, tree, "product-row-1")
	if row.Role != "other" || !row.IsInteractive {
		t.Errorf("an identified GenericElement is an interactive other, got %s interactive=%v", row.Role, row.IsInteractive)
	}
	tab := byIdentifier(t, tree, "tab-network")
	if tab.Role != "tab" || tab.Name != "Network" {
		t.Errorf("React Navigation tab = %s %q, want tab \"Network\"", tab.Role, tab.Name)
	}
	if selected := byIdentifier(t, tree, "tab-products"); selected.State["selected"] != "true" {
		t.Errorf("Selected trait not in state: %v", selected.State)
	}
	for _, n := range tree.Nodes {
		if n.Name == "Unidentified text" && n.IsInteractive {
			t.Error("an unidentified GenericElement without a tappable trait is not interactive")
		}
	}
}

func TestNormalizeDropsKeyboardKeys(t *testing.T) {
	tree := Normalize(loadDescribe(t, "describe-rn-keyboard"))
	var keyboard *sightmap.ComponentNode
	for _, n := range tree.Nodes {
		switch n.Role {
		case "key":
			t.Errorf("key %q survived normalization", n.Name)
		case "keyboard":
			keyboard = n
		}
		if n.Name == "return" {
			t.Error("a KeyboardKey button survived normalization")
		}
	}
	if keyboard == nil || len(keyboard.Children) != 0 || keyboard.IsInteractive {
		t.Fatalf("want one childless, non-interactive keyboard node, got %+v", keyboard)
	}
	card := tree.Nodes[2]
	if card.Name != "Spring Meetup, 64 members" || !card.IsInteractive {
		t.Errorf("a labelled other with the Button trait is interactive: %q %v", card.Name, card.IsInteractive)
	}
}

func TestAnnotateInfersViewAndExtractsProperties(t *testing.T) {
	s := snapshot(t, "describe-simple", "corpus", "")
	if s.ViewName() != "Simple" || !s.ViewInferred {
		t.Fatalf("view = %q inferred=%v", s.ViewName(), s.ViewInferred)
	}
	toggle := byIdentifier(t, s.Tree, "simple.toggle")
	if s.Component(toggle) != "NotificationsToggle" || s.Props[toggle.Id]["on"] != "true" {
		t.Errorf("toggle = %q %v", s.Component(toggle), s.Props[toggle.Id])
	}
	tab := byIdentifier(t, s.Tree, "1.circle")
	if s.Component(tab) != "Tab" || s.Props[tab.Id]["title"] != "Simple" {
		t.Errorf("tab = %q %v", s.Component(tab), s.Props[tab.Id])
	}
	text := s.Render(RenderOpts{})
	for _, want := range []string{
		"[View: Simple] (inferred)",
		"[NotificationsToggle on=\"true\" \"Notifications\"]",
		"[Memory]\n- The tab bar is present on every screen.",
		"[Coverage] (visible only)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("render lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "/simple") {
		t.Error("the placeholder route leaked into the snapshot")
	}
}

func TestAnnotateNamedViewAndUnknownView(t *testing.T) {
	s := snapshot(t, "describe-simple", "corpus", "Heavy")
	if s.ViewName() != "Heavy" || s.ViewInferred {
		t.Fatalf("view = %q inferred=%v", s.ViewName(), s.ViewInferred)
	}
	if got := s.Component(byIdentifier(t, s.Tree, "simple.primary")); got != "" {
		t.Errorf("Simple's component applied under the Heavy view: %q", got)
	}
	_, err := Annotate(Normalize(loadDescribe(t, "describe-simple")), loadCorpus(t, "corpus"), nil, "Nope")
	if Code(err) != "unknown_view" {
		t.Errorf("err = %v", err)
	}
}

func TestAnnotateHeavyComponentProperties(t *testing.T) {
	s := snapshot(t, "describe-heavy", "corpus", "")
	if s.ViewName() != "Heavy" {
		t.Fatalf("view = %q", s.ViewName())
	}
	rows := 0
	for _, n := range s.Nodes {
		if s.Component(n) != "ProductRow" {
			continue
		}
		rows++
		p := s.Props[n.Id]
		if !strings.HasPrefix(p["title"], "Row") || !strings.HasPrefix(p["price"], "$") || p["favoritable"] == "" {
			t.Errorf("row %s props = %v", n.Id, p)
		}
	}
	if rows == 0 {
		t.Fatal("no ProductRow matched")
	}
}

func TestAnnotateReactNativeTestidAlias(t *testing.T) {
	s := snapshot(t, "describe-rn", "rn-corpus", "")
	if s.ViewName() != "Products" {
		t.Fatalf("view = %q", s.ViewName())
	}
	row := byIdentifier(t, s.Tree, "product-row-15")
	if s.Component(row) != "ProductRow" || s.Props[row.Id]["number"] != "15" {
		t.Errorf("row = %q %v", s.Component(row), s.Props[row.Id])
	}
	if got := s.Props[byIdentifier(t, s.Tree, "tab-account").Id]["title"]; got != "Account" {
		t.Errorf("tab title = %q", got)
	}
}

func TestRenderMarksTheSelectedElement(t *testing.T) {
	s := snapshot(t, "describe-rn", "", "")
	if l := s.line(byIdentifier(t, s.Tree, "tab-products")); !strings.HasSuffix(l, " (selected)") {
		t.Errorf("selected tab line = %q", l)
	}
	if l := s.line(byIdentifier(t, s.Tree, "tab-network")); strings.Contains(l, "(selected)") {
		t.Errorf("unselected tab line = %q", l)
	}
}

func TestSocketPathFitsWhenTheTempDirIsLong(t *testing.T) {
	t.Setenv("TMPDIR", "/private/tmp/"+strings.Repeat("sandbox/", 12))
	if p := SocketPath(".sightmap", "A2E36025-0000-0000-0000-000000000000"); len(p) > maxSocketPath || !strings.HasPrefix(p, "/tmp/") {
		t.Errorf("socket path = %q (%d bytes)", p, len(p))
	}
}

// The snapshot is matched by sightmap's own matcher, so a tree written with
// --tree-out and matched offline names every node the same way.
func TestTreeJSONRoundTripMatchesTheSame(t *testing.T) {
	s := snapshot(t, "describe-heavy", "corpus", "")
	data, err := json.Marshal(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	var root sightmap.ComponentNode
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	offline := map[string]string{}
	for n, cm := range match.NewMatcher(loadCorpus(t, "corpus")).MatchView(&root, s.View) {
		offline[n.Id] = cm.Name
	}
	for n, cm := range s.Matches {
		if offline[n.Id] != cm.Name {
			t.Errorf("node %s: snapshot %q, offline %q", n.Id, cm.Name, offline[n.Id])
		}
	}
	if len(offline) != len(s.Matches) {
		t.Errorf("offline matched %d nodes, snapshot %d", len(offline), len(s.Matches))
	}
}

func TestResolveTargets(t *testing.T) {
	s := snapshot(t, "describe-simple", "corpus", "")
	primary := byIdentifier(t, s.Tree, "simple.primary")
	for _, target := range []string{primary.Id, "simple.primary", "PrimaryButton", `button[identifier="simple.primary"]`, `[text="Primary"]`} {
		n, err := Resolve(s, target)
		if err != nil || n != primary {
			t.Errorf("Resolve(%q) = %v, %v", target, n, err)
		}
	}
	if n, err := Resolve(s, `Tab[title="Heavy"]`); err != nil || Identifier(n) != "2.circle" {
		t.Errorf("Tab[title=Heavy] = %v, %v", n, err)
	}
	if n, err := Resolve(s, "TabBar Tab#2"); err != nil || n.Name != "Deep" {
		t.Errorf("TabBar Tab#2 = %v, %v", n, err)
	}
	if _, err := Resolve(s, "Tab"); Code(err) != "ambiguous" || !strings.Contains(err.Error(), "\"Eager\"") {
		t.Errorf("Tab: %v", err)
	}
	if n, err := Resolve(s, "tab#3"); err != nil || n.Name != "Eager" {
		t.Errorf("tab#3 = %v, %v", n, err)
	}
	if _, err := Resolve(s, "CheckoutButton"); Code(err) != "not_found" || !strings.Contains(err.Error(), "no CheckoutButton component") {
		t.Errorf("CheckoutButton: %v", err)
	}
	if _, err := Resolve(s, "999"); Code(err) != "not_found" {
		t.Errorf("999: %v", err)
	}
	if _, err := Resolve(s, "button[[["); Code(err) != "bad_query" {
		t.Errorf("bad selector: %v", err)
	}
}

func TestAssertActionable(t *testing.T) {
	s := snapshot(t, "describe-heavy", "corpus", "")
	var below *sightmap.ComponentNode
	for _, n := range s.Nodes {
		if n.IsInteractive && !n.IsVisible {
			below = n
			break
		}
	}
	if below == nil {
		t.Fatal("fixture has no off-screen interactive node")
	}
	err := s.AssertActionable(below, below.Id)
	if Code(err) != "offscreen" || !strings.Contains(AsError(err).Hint, "scroll down --until") {
		t.Errorf("off-screen: %v (%s)", err, AsError(err).Hint)
	}

	d := loadDescribe(t, "describe-rn-keyboard")
	d.Root.Children = d.Root.Children[:len(d.Root.Children)-1] // without the floating button
	kb, _ := Annotate(Normalize(d), loadCorpus(t, "rn-corpus"), nil, "")
	join := byIdentifier(t, kb.Tree, "join-button")
	if err := kb.AssertActionable(join, "join-button"); Code(err) != "covered" || !strings.Contains(err.Error(), "keyboard") {
		t.Errorf("under the keyboard: %v", err)
	}
	search := byIdentifier(t, kb.Tree, "search")
	if err := kb.AssertActionable(search, "search"); err != nil {
		t.Errorf("search field: %v", err)
	}

	rn := snapshot(t, "describe-rn", "rn-corpus", "")
	row := byIdentifier(t, rn.Tree, "product-row-15")
	if err := rn.AssertActionable(row, "product-row-15"); Code(err) != "covered" || !strings.Contains(err.Error(), "tabbar") {
		t.Errorf("a row behind React Navigation tabs: %v", err)
	}
	if err := rn.AssertActionable(byIdentifier(t, rn.Tree, "tab-network"), "tab-network"); err != nil {
		t.Errorf("a tab is never covered by its own band: %v", err)
	}
}

func TestFloatingDevMenuButton(t *testing.T) {
	kb := snapshot(t, "describe-rn-keyboard", "rn-corpus", "")
	floating := kb.floatingControls()
	if len(floating) != 1 || floating[0].Bounds.Width != 60 {
		t.Fatalf("floating = %v", floating)
	}
	join := byIdentifier(t, kb.Tree, "join-button")
	kb.Nodes = slicesDelete(kb.Nodes, "keyboard")
	err := kb.AssertActionable(join, "join-button")
	if Code(err) != "covered" || !strings.Contains(err.Error(), "dev-menu") {
		t.Errorf("near the floating button: %v", err)
	}
}

func slicesDelete(nodes []*sightmap.ComponentNode, role string) []*sightmap.ComponentNode {
	var out []*sightmap.ComponentNode
	for _, n := range nodes {
		if n.Role != role {
			out = append(out, n)
		}
	}
	return out
}

func TestRelocateSnapshotID(t *testing.T) {
	s := snapshot(t, "describe-simple", "corpus", "")
	primary := byIdentifier(t, s.Tree, "simple.primary")
	fresh := snapshot(t, "describe-simple", "corpus", "")
	n, err := Relocate(fresh, primary, primary.Id)
	if err != nil || Identifier(n) != "simple.primary" {
		t.Errorf("Relocate = %v, %v", n, err)
	}
	rn := snapshot(t, "describe-rn", "rn-corpus", "")
	if _, err := Relocate(rn, primary, primary.Id); Code(err) != "stale_target" {
		t.Errorf("gone: %v", err)
	}
}

func TestSparseNote(t *testing.T) {
	d := &Describe{Screen: Size{W: 400, H: 800}, Root: &RawElement{Type: "Application", Frame: Frame{W: 400, H: 800}, Children: []*RawElement{
		{Type: "Button", Frame: Frame{X: 330, Y: 700, W: 50, H: 50}},
	}}}
	s, _ := Annotate(Normalize(d), nil, nil, "")
	note := s.sparseNote("UDID", true)
	if len(note) == 0 || !strings.Contains(note[0], "small floating element") {
		t.Fatalf("note = %v", note)
	}
	if strings.Contains(strings.Join(note, "\n"), "App accessibility is off") {
		t.Error("accessibility advice given although it is on")
	}
	if !strings.Contains(s.Render(RenderOpts{UDID: "UDID"}), "[Mostly empty]") {
		t.Error("render lacks [Mostly empty]")
	}
}

// A snapshot result decodes back into its tree (the CLI and library callers
// read `snapshot --json`); this must hold under encoding/json/v2 too
// (GOEXPERIMENT=jsonv2 before Go 1.27, the default after).
func TestSnapshotResultJSONRoundTrip(t *testing.T) {
	s := snapshot(t, "describe-heavy", "corpus", "")
	data, err := json.Marshal(SnapshotResult{Text: "x", Tree: s.AnnotatedTree()})
	if err != nil {
		t.Fatal(err)
	}
	var r SnapshotResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if r.Tree == nil || r.Tree.ComponentNode == nil || r.Tree.Id != "1" || len(r.Tree.Children) != len(s.Root.Children) {
		t.Fatalf("tree did not round-trip: %+v", r.Tree)
	}
	var rows int
	var walk func(n *AnnotatedNode)
	walk = func(n *AnnotatedNode) {
		if n.Component == "ProductRow" && n.Props["title"] != "" {
			rows++
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(r.Tree)
	if rows == 0 {
		t.Error("no ProductRow with props survived the round trip")
	}
}

// A custom tab bar of plain buttons (no tab role, no tab bar container) drawn
// over a list covers the rows under it.
func TestCustomBarDrawnOverAListCoversRows(t *testing.T) {
	row := func(i int, y float64) *RawElement {
		return &RawElement{Type: "Button", Identifier: fmt.Sprintf("participant-row-%d", i), Label: fmt.Sprintf("Row %d", i), Frame: Frame{X: 0, Y: y, W: 400, H: 60}}
	}
	d := &Describe{Screen: Size{W: 400, H: 800}, Root: &RawElement{Type: "Application", Frame: Frame{W: 400, H: 800}, Children: []*RawElement{
		row(1, 100), row(25, 720),
		{Type: "Button", Identifier: "leaderboard-tab", Label: "Leaderboard", Frame: Frame{X: 20, Y: 710, W: 170, H: 70}},
		{Type: "Button", Identifier: "stats-tab", Label: "Stats", Frame: Frame{X: 190, Y: 710, W: 190, H: 70}},
	}}}
	s, _ := Annotate(Normalize(d), nil, nil, "")
	err := s.AssertActionable(byIdentifier(t, s.Tree, "participant-row-25"), "participant-row-25")
	if err == nil || Code(err) != "covered" || !strings.Contains(err.Error(), "stats-tab") || !strings.Contains(AsError(err).Hint, "scroll down") {
		t.Fatalf("row under the bar: %v", err)
	}
	for _, id := range []string{"participant-row-1", "stats-tab"} {
		if err := s.AssertActionable(byIdentifier(t, s.Tree, id), id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

// The Expo dev client's floating gear reports its SF Symbol name as its
// identifier.
func TestFloatingExpoGear(t *testing.T) {
	d := &Describe{Screen: Size{W: 400, H: 800}, Root: &RawElement{Type: "Application", Frame: Frame{W: 400, H: 800}, Children: []*RawElement{
		{Type: "Button", Identifier: "topbar-avatar-button", Label: "Profile", Frame: Frame{X: 330, Y: 60, W: 50, H: 40}},
		{Type: "Button", Identifier: "settings-button", Frame: Frame{X: 10, Y: 60, W: 44, H: 44}, Children: []*RawElement{
			{Type: "Image", Identifier: "gearshape", Frame: Frame{X: 16, Y: 66, W: 32, H: 32}},
		}},
		{Type: "Image", Identifier: "gearshape.fill", Frame: Frame{X: 340, Y: 100, W: 44, H: 44}},
	}}}
	s, _ := Annotate(Normalize(d), nil, nil, "")
	floating := s.floatingControls()
	if len(floating) != 1 || Identifier(floating[0]) != "gearshape.fill" {
		t.Fatalf("floating = %v", floating)
	}
	if err := s.AssertActionable(byIdentifier(t, s.Tree, "topbar-avatar-button"), "topbar-avatar-button"); err == nil || !strings.Contains(err.Error(), "dev-menu") {
		t.Errorf("avatar next to the gear: %v", err)
	}
	if err := s.AssertActionable(byIdentifier(t, s.Tree, "settings-button"), "settings-button"); err != nil {
		t.Errorf("the app's own gear button: %v", err)
	}
}
