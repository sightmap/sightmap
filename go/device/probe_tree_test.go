package device

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/projection"
)

// burritoDump reads a dump the probe took of Burrito, a fictional ordering
// app, on one of its screens.
func burritoDump(t *testing.T, screen string) *ProbeDump {
	t.Helper()
	b, err := os.ReadFile("testdata/probe/burrito-" + screen + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Result ProbeDump `json:"result"`
	}
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	return &rep.Result
}

func burritoTree(t *testing.T, screen string) *Tree {
	t.Helper()
	d := burritoDump(t, screen)
	root := ProbeTree(&RawElement{Type: "Application", Label: "Burrito"}, d.Meta.AX, d.Meta.Screen)
	if root == nil {
		t.Fatal("no tree")
	}
	return Normalize(&Describe{Screen: d.Meta.Screen, Root: root})
}

// outline renders a tree as one line per node: role, #identifier, "label".
func outline(n *RawElement, depth int, out *[]string) {
	s := strings.Repeat("  ", depth) + strings.ToLower(n.Type)
	if n.Identifier != "" {
		s += "#" + n.Identifier
	}
	if n.Label != "" {
		s += fmt.Sprintf(" %q", n.Label)
	}
	*out = append(*out, s)
	for _, c := range n.Children {
		outline(c, depth+1, out)
	}
}

func TestProbeTreeBurrito(t *testing.T) {
	for _, screen := range []string{"01-signin", "02-menu", "03-orders", "04-account", "05-cart"} {
		d := burritoDump(t, screen)
		root := ProbeTree(&RawElement{Type: "Application", Label: "Burrito"}, d.Meta.AX, d.Meta.Screen)
		var lines []string
		outline(root, 0, &lines)
		t.Logf("%s\n%s", screen, strings.Join(lines, "\n"))
	}
}

// TestProbeTreeBurritoMenu pins the menu: the app's own container ids
// (`menu.list`, `menu.card.*`), which the out-of-process tree cannot see, come
// with it; UIKit's duplicate tab bar, its dimming views and its navigation
// bar's title-derived id do not.
func TestProbeTreeBurritoMenu(t *testing.T) {
	d := burritoDump(t, "02-menu")
	root := ProbeTree(&RawElement{Type: "Application", Label: "Burrito"}, d.Meta.AX, d.Meta.Screen)
	var lines []string
	outline(root, 0, &lines)
	got := strings.Join(lines, "\n")
	for _, want := range []string{"group#menu.list", "  group#menu.card.classic-burrito", "button#menu.tab.all", "    button#menu.cart", "button#tab.menu"} {
		if !strings.Contains(got, strings.TrimLeft(want, " ")) {
			t.Errorf("tree has no %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"AdditionalDimmingOverlay", "#🌯 Burrito Co."} {
		if strings.Contains(got, bad) {
			t.Errorf("tree has %q:\n%s", bad, got)
		}
	}
	if n := strings.Count(got, "button#tab.menu"); n != 1 {
		t.Errorf("tab.menu appears %d times, want 1", n)
	}
}

// TestProbeTreeFeedsNormalize checks the elements Normalize reads: a switch is a
// switch, a tab a tab, and the password field keeps its secure type.
func TestProbeTreeFeedsNormalize(t *testing.T) {
	tr := burritoTree(t, "01-signin")
	var tags []string
	for _, n := range tr.Nodes {
		tags = append(tags, n.Element.Tag+"#"+n.Element.Id)
	}
	if !strings.Contains(strings.Join(tags, " "), "securetextfield#signin.password") {
		t.Errorf("no secure field: %v", tags)
	}
	acct := burritoTree(t, "04-account")
	sw := 0
	for _, n := range acct.Nodes {
		if n.Element.Tag == "switch" {
			sw++
		}
	}
	if sw != 2 {
		t.Errorf("account has %d switches, want 2", sw)
	}
}

// TestProbeTreeMatchesLidar compares, per Burrito screen, the identifying nodes
// (identifier, else label) of the tree device builds from the probe with those of
// the tree lidar builds from a Fullstory capture of the same screen, after the
// same projection. Whatever differs is listed, with its reason; anything else
// fails.
func TestProbeTreeMatchesLidar(t *testing.T) {
	// Differences in what each side can see, not in the rules.
	known := map[string][]string{
		"02-menu": {
			"device only: text:Three soft corn tortillas with sofritas, roasted chili-corn salsa, fresh guacamole, and shredded cheese.",
			"lidar only: text:Three soft corn tortillas with sofritas, roasted chili-corn salsa, fresh guacamole, and shredded ...", // the capture cuts a long text
			"lidar only: id:🌯 Burrito Co.", // UIKit's id for the navigation bar, which lidar's collapse moved onto a wrapper
			"lidar only: text:Cart",        // a wrapper lidar names after its bar item
		},
		"05-cart": {
			"lidar only: id:🌯 Burrito Co.",    // UIKit's id for the navigation bar
			"lidar only: text:🌯 Burrito Co.",  // the back button: "Back" in the capture, the previous title in the tree
			"lidar only: id:chevron.backward", // the back button's image, which accessibility hides
			"lidar only: id:cart.checkout",    // a disabled button the capture also reports as a text
		},
	}
	for _, screen := range []string{"01-signin", "02-menu", "03-orders", "04-account", "05-cart"} {
		t.Run(screen, func(t *testing.T) {
			d := burritoDump(t, screen)
			root := ProbeTree(&RawElement{Type: "Application", Label: "Burrito"}, d.Meta.AX, d.Meta.Screen)
			dev := map[string]int{}
			var walk func(e *RawElement)
			walk = func(e *RawElement) {
				switch {
				case e.Identifier != "":
					dev["id:"+e.Identifier]++
				case e.Label != "":
					dev["text:"+e.Label]++
				}
				for _, c := range e.Children {
					walk(c)
				}
			}
			for _, c := range root.Children {
				walk(c)
			}
			lid := map[string]int{}
			b, err := os.ReadFile("../projection/testdata/burrito/" + screen + ".lidar.out.json")
			if err != nil {
				t.Fatal(err)
			}
			var lt projectionNode
			if err := json.Unmarshal(b, &lt); err != nil {
				t.Fatal(err)
			}
			var lw func(n *projectionNode)
			lw = func(n *projectionNode) {
				switch {
				// UIKit names a navigation bar after its title; the lidar adapter, which
				// does not exist yet, will drop that id as device's adapter does.
				case n.Kind == projection.KindNavigation && n.Text != "":
					lid["text:"+n.Text]++
				case n.ID != "":
					lid["id:"+n.ID]++
				case n.Text != "":
					lid["text:"+n.Text]++
				}
				for _, k := range n.Kids {
					lw(k)
				}
			}
			for _, k := range lt.Kids {
				lw(k)
			}
			want := map[string]bool{}
			for _, k := range known[screen] {
				want[k] = true
			}
			var diffs []string
			for k, n := range dev {
				if n > lid[k] {
					diffs = append(diffs, "device only: "+k)
				}
			}
			for k, n := range lid {
				if n > dev[k] {
					diffs = append(diffs, "lidar only: "+k)
				}
			}
			for _, d := range diffs {
				if !want[d] {
					t.Errorf("unexplained: %q", d)
				}
			}
			t.Logf("%d device, %d lidar, %d differences", len(dev), len(lid), len(diffs))
		})
	}
}

type projectionNode struct {
	Kind string            `json:"kind"`
	ID   string            `json:"id"`
	Text string            `json:"text"`
	Kids []*projectionNode `json:"kids"`
}
