package projection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, path string) *Node {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var n Node
	if err := json.Unmarshal(b, &n); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return &n
}

// normalize makes nil and empty Kids equal for comparison.
func normalize(n *Node) {
	if len(n.Kids) == 0 {
		n.Kids = nil
	}
	for _, k := range n.Kids {
		normalize(k)
	}
}

// TestBurritoFixtures projects each screen's two producers' inputs (the probe's
// in-process accessibility tree, and a Fullstory capture read by lidar) and
// checks the result against the expected tree. The expected trees were produced
// by the reference implementation (a Python prototype) and reviewed by hand;
// they differ between producers only where one sees something the other cannot
// (see TestBurritoProducersAgree).
func TestBurritoFixtures(t *testing.T) {
	ins, _ := filepath.Glob("testdata/burrito/*.in.json")
	if len(ins) == 0 {
		t.Fatal("no fixtures")
	}
	for _, in := range ins {
		name := strings.TrimSuffix(filepath.Base(in), ".in.json")
		t.Run(name, func(t *testing.T) {
			got := Project(load(t, in))
			want := load(t, strings.TrimSuffix(in, ".in.json")+".out.json")
			normalize(got)
			normalize(want)
			if !reflect.DeepEqual(got, want) {
				g, _ := json.MarshalIndent(got, "", " ")
				w, _ := json.MarshalIndent(want, "", " ")
				t.Errorf("projection differs\n got: %s\nwant: %s", g, w)
			}
		})
	}
}

func TestProjectLeavesInputAlone(t *testing.T) {
	in := &Node{Kind: KindRoot, Kids: []*Node{{Kind: KindGroup, Kids: []*Node{{Kind: KindText, Text: "a"}, {Kind: KindText, Text: "b"}}}}}
	Project(in)
	if len(in.Kids) != 1 || in.Kids[0].Kind != KindGroup || len(in.Kids[0].Kids) != 2 {
		t.Errorf("input was modified: %+v", in.Kids[0])
	}
}

func TestRules(t *testing.T) {
	root := func(kids ...*Node) *Node { return &Node{Kind: KindRoot, Kids: kids} }
	tests := []struct {
		name string
		in   *Node
		want []string // kind[#id] "text", depth-first, indented by depth
	}{
		{"unnamed group splices", root(&Node{Kind: KindGroup, Kids: []*Node{{Kind: KindText, Text: "a"}, {Kind: KindText, Text: "b"}}}),
			[]string{`text "a"`, `text "b"`}},
		{"id group hands id to sole anonymous child", root(&Node{Kind: KindGroup, ID: "x", Kids: []*Node{{Kind: KindButton, Text: "Go"}}}),
			[]string{`button#x "Go"`}},
		{"control takes its text child", root(&Node{Kind: KindButton, ID: "b", Kids: []*Node{{Kind: KindText, Text: "Buy"}}}),
			[]string{`button#b "Buy"`}},
		{"control icon is part of the control", root(&Node{Kind: KindButton, Text: "Back", Kids: []*Node{{Kind: KindImage}}}),
			[]string{`button "Back"`}},
		{"unnamed list splices", root(&Node{Kind: KindList, Kids: []*Node{{Kind: KindText, ID: "t", Text: "a"}}}),
			[]string{`text#t "a"`}},
		{"navigation takes its heading", root(&Node{Kind: KindNavigation, Kids: []*Node{{Kind: KindHeading, Text: "Cart"}}}),
			[]string{`navigation "Cart"`}},
		{"nodes with an app id stay", root(&Node{Kind: KindGroup, ID: "menu.list", Kids: []*Node{{Kind: KindText, Text: "a"}, {Kind: KindText, Text: "b"}}}),
			[]string{`group#menu.list`, `  text "a"`, `  text "b"`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := render(Project(tc.in), -1, nil)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func render(n *Node, depth int, out []string) []string {
	if n.Kind != KindRoot {
		s := strings.Repeat("  ", depth) + n.Kind
		if n.ID != "" {
			s += "#" + n.ID
		}
		if n.Text != "" {
			s += ` "` + n.Text + `"`
		}
		out = append(out, s)
	}
	for _, k := range n.Kids {
		out = render(k, depth+1, out)
	}
	return out
}

// keys is the multiset of identifying keys in a tree: a node's id, else its text.
func keys(n *Node, m map[string]int) {
	if n.Kind != KindRoot {
		switch {
		case n.ID != "":
			m["id:"+n.ID]++
		case n.Text != "":
			m["text:"+n.Text]++
		}
	}
	for _, k := range n.Kids {
		keys(k, m)
	}
}

// residuals are the keys one producer has and the other does not, per screen,
// and why. Each is a difference in what the producer can see, not a rule gap.
// Ids that UIKit derives from a title (the navigation bar's) are not app ids;
// a producer adapter should drop them, and until the adapters exist the
// fixtures carry them as they came off the screen.
var residuals = map[string][]string{
	"02-menu": {
		"device only: text:Three soft corn tortillas with…", // AX truncates a long label; capture has it whole
		"lidar only: text:🌯 Burrito Co.",                    // navigation title drawn by UIKit
		"lidar only: text:Three soft corn tortillas with",
		"lidar only: text:Cart",
	},
	"05-cart": {
		"device only: id:Your Cart",      // UIKit-derived navigation id
		"lidar only: id:🌯 Burrito Co.",   // UIKit-derived navigation id
		"lidar only: text:🌯 Burrito Co.", // back button: AX says the previous title, capture says "Back"
		"lidar only: text:Your Cart",
		"lidar only: id:chevron.backward", // the back button's image, which AX hides
	},
}

// TestBurritoProducersAgree is the point of the package: the tree from the
// simulator's in-process accessibility data and the tree from a captured
// session of the same screen have the same identifying nodes, except for the
// listed residuals.
func TestBurritoProducersAgree(t *testing.T) {
	screens := []string{"01-signin", "02-menu", "03-orders", "04-account", "05-cart"}
	for _, s := range screens {
		t.Run(s, func(t *testing.T) {
			dev, lid := map[string]int{}, map[string]int{}
			keys(Project(load(t, "testdata/burrito/"+s+".device.in.json")), dev)
			keys(Project(load(t, "testdata/burrito/"+s+".lidar.in.json")), lid)
			var diff []string
			for k, n := range dev {
				if n > lid[k] {
					diff = append(diff, "device only: "+k)
				}
			}
			for k, n := range lid {
				if n > dev[k] {
					diff = append(diff, "lidar only: "+k)
				}
			}
			t.Logf("%d device keys, %d lidar keys, %d residuals", len(dev), len(lid), len(diff))
			want := map[string]bool{}
			for _, r := range residuals[s] {
				want[r] = true
			}
			for _, d := range diff {
				if !want[d] && !prefixed(want, d) {
					t.Errorf("unexplained difference: %s", d)
				}
			}
		})
	}
}

// prefixed reports whether d matches a residual up to a truncated label: AX
// shortens long labels with an ellipsis, so the text key differs in its tail.
func prefixed(want map[string]bool, d string) bool {
	for w := range want {
		a, b := strings.TrimSuffix(w, "…"), strings.TrimSuffix(d, "…")
		if len(a) > 20 && strings.HasPrefix(b, a[:20]) {
			return true
		}
	}
	return false
}
