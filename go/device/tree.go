package device

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Describe is the helper's `describe` result: the frontmost app's accessibility
// tree in screen points, top-left origin.
type Describe struct {
	Screen Size        `json:"screen"`
	Root   *RawElement `json:"root"`
	// Overlay is true when the tree belongs to a system sheet from another
	// process (e.g. "Save Password?") rather than to the app.
	Overlay bool `json:"overlay,omitempty"`
	// Settled is true when the reader waited for the screen to go idle before
	// reading it, as `uiautomator dump` does, so one read stands for two that
	// agree.
	Settled bool `json:"settled,omitempty"`
	// StatusBar is the status bar's height in points when the reader knows
	// it (Android); zero leaves it to systemTop's per-device guess.
	StatusBar float64 `json:"statusBar,omitempty"`
}

// Size is a screen size in points.
type Size struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Frame is an element's frame in points.
type Frame struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// RawElement is one accessibility element as the helper reports it.
type RawElement struct {
	Role       string        `json:"role"`
	Subrole    string        `json:"subrole"`
	Type       string        `json:"type"`
	Label      string        `json:"label"`
	Identifier string        `json:"identifier"`
	Value      Value         `json:"value"`
	Frame      Frame         `json:"frame"`
	Enabled    *bool         `json:"enabled"`
	Traits     []string      `json:"traits"`
	Children   []*RawElement `json:"children"`
}

// Value is an element's accessibility value. The helper sends a string or null;
// older recordings carry a switch's value as a number, read here as its text.
type Value string

func (v *Value) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*v = Value(s)
		return nil
	}
	if string(data) == "null" {
		*v = ""
		return nil
	}
	*v = Value(strings.Trim(string(data), `"`))
	return nil
}

// Tree is a normalized accessibility tree in sightmap's ComponentNode shape, so
// the corpus matcher, coverage and the offline tools read it like a web page.
// Each element is one node:
//
//	Role, Element.Tag  the normalized type: button, text, textfield, switch, tab, cell, other, …
//	Element.Attrs      identifier (testid is an alias), text (the label), value, disabled
//	Element.Id         the identifier, when it is a valid CSS identifier
//	Name               the label
//	Text               the label, or the children's text joined
//	State              disabled, selected (Selected trait), checked (a switch's 1 / 0)
//
// Ids are per tree: "1" is the root, in pre-order.
type Tree struct {
	Screen    Size
	Root      *sightmap.ComponentNode
	Nodes     []*sightmap.ComponentNode
	Overlay   bool
	Settled   bool    // see Describe.Settled
	StatusBar float64 // see Describe.StatusBar

	parent map[*sightmap.ComponentNode]*sightmap.ComponentNode
	traits map[*sightmap.ComponentNode][]string
}

var tagByType = map[string]string{
	"application":       "application",
	"window":            "window",
	"group":             "other",
	"genericelement":    "other",
	"other":             "other",
	"button":            "button",
	"statictext":        "text",
	"text":              "text",
	"textfield":         "textfield",
	"securetextfield":   "securetextfield",
	"searchfield":       "searchfield",
	"textview":          "textview",
	"textarea":          "textview",
	"image":             "image",
	"icon":              "image",
	"cell":              "cell",
	"table":             "table",
	"collectionview":    "collection",
	"scrollarea":        "scrollview",
	"scrollview":        "scrollview",
	"heading":           "heading",
	"link":              "link",
	"checkbox":          "checkbox",
	"switch":            "switch",
	"toggle":            "switch",
	"radiobutton":       "radio",
	"slider":            "slider",
	"stepper":           "stepper",
	"incrementor":       "stepper",
	"segmentedcontrol":  "segmentedcontrol",
	"picker":            "picker",
	"pickerwheel":       "pickerwheel",
	"popupbutton":       "popup",
	"menu":              "menu",
	"menuitem":          "menuitem",
	"navigationbar":     "navbar",
	"tabbar":            "tabbar",
	"toolbar":           "toolbar",
	"alert":             "alert",
	"sheet":             "sheet",
	"keyboard":          "keyboard",
	"key":               "key",
	"progressindicator": "progress",
	"activityindicator": "progress",
	"webview":           "webview",
}

var tagBySubrole = map[string]string{
	"AXSwitch":          "switch",
	"AXTabButton":       "tab",
	"AXSearchField":     "searchfield",
	"AXSecureTextField": "securetextfield",
}

var interactiveTags = map[string]bool{
	"button": true, "link": true, "cell": true, "textfield": true, "securetextfield": true,
	"searchfield": true, "textview": true, "checkbox": true, "switch": true, "radio": true,
	"tab": true, "slider": true, "stepper": true, "segmentedcontrol": true, "picker": true,
	"pickerwheel": true, "popup": true, "menuitem": true,
}

var tappableTraits = []string{"Button", "Link", "Adjustable"}

const maxText = 300

func tagFor(raw *RawElement) string {
	if t, ok := tagBySubrole[raw.Subrole]; ok {
		return t
	}
	name := raw.Type
	if name == "" {
		name = raw.Role
	}
	if name == "" {
		name = "other"
	}
	name = strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(name, "XCUIElementType"), "AX"))
	if t, ok := tagByType[name]; ok {
		return t
	}
	return name
}

// React Native reports accessibilityRole="tab" (React Navigation's tab buttons)
// as a button whose label carries VoiceOver's "tab, N of M" suffix.
var rnTabLabel = regexp.MustCompile(`^(.*), tab, \d+ of \d+$`)

func rnTabTitle(raw *RawElement) (string, bool) {
	m := rnTabLabel.FindStringSubmatch(raw.Label)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func isWrapper(raw *RawElement, tag string) bool {
	return tag == "other" && raw.Label == "" && raw.Identifier == "" && raw.Value == ""
}

// The system keyboard is not part of the app: it never matches components and
// never counts toward coverage. Its container stays as one childless node so
// actions can tell what it covers.
func isKeyboard(raw *RawElement, tag string) bool {
	return tag == "keyboard" || tag == "key" || slices.Contains(raw.Traits, "KeyboardKey")
}

// isInteractive: an interactive role, or an `other` that is most likely
// tappable. React Native's accessible Views (role-less Pressables included) are
// GenericElements, so one with a testID counts; so does a labelled or
// identified `other` whose traits say it can be tapped.
func isInteractive(raw *RawElement, tag string) bool {
	if interactiveTags[tag] {
		return true
	}
	if tag != "other" {
		return false
	}
	kind := strings.ToLower(strings.TrimPrefix(cmpOr(raw.Type, raw.Role), "AX"))
	if kind == "genericelement" && raw.Identifier != "" {
		return true
	}
	return (raw.Label != "" || raw.Identifier != "") && slices.ContainsFunc(raw.Traits, func(t string) bool { return slices.Contains(tappableTraits, t) })
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type collapsed struct {
	raw      *RawElement
	tag      string
	children []collapsed
}

// collapse drops unlabeled single-child and empty `other` wrappers (their child
// takes their place) and reduces the keyboard to its container.
func collapse(raw *RawElement, isRoot bool) []collapsed {
	tag := tagFor(raw)
	if _, ok := rnTabTitle(raw); ok {
		tag = "tab"
	}
	if isKeyboard(raw, tag) {
		if tag != "keyboard" {
			return nil
		}
		return []collapsed{{raw: raw, tag: tag}}
	}
	var children []collapsed
	for _, c := range raw.Children {
		children = append(children, collapse(c, false)...)
	}
	if !isRoot && isWrapper(raw, tag) && len(children) <= 1 {
		return children
	}
	return []collapsed{{raw: raw, tag: tag, children: children}}
}

func intersects(f Frame, s Size) bool {
	return f.W > 0 && f.H > 0 && f.X < s.W && f.Y < s.H && f.X+f.W > 0 && f.Y+f.H > 0
}

var cssIdent = regexp.MustCompile(`^-?[A-Za-z_][A-Za-z0-9_-]*$`)

// Normalize converts a describe result into a Tree.
func Normalize(d *Describe) *Tree {
	t := &Tree{
		Screen:    d.Screen,
		Overlay:   d.Overlay,
		Settled:   d.Settled,
		StatusBar: d.StatusBar,
		parent:    map[*sightmap.ComponentNode]*sightmap.ComponentNode{},
		traits:    map[*sightmap.ComponentNode][]string{},
	}
	if d.Root == nil {
		d.Root = &RawElement{}
	}
	if t.Screen.W == 0 || t.Screen.H == 0 {
		t.Screen = Size{W: d.Root.Frame.W, H: d.Root.Frame.H}
	}
	top := collapse(d.Root, true)
	t.Root = t.build(top[0], nil, 1)
	return t
}

func (t *Tree) build(c collapsed, parent *sightmap.ComponentNode, nth int) *sightmap.ComponentNode {
	raw := c.raw
	label := raw.Label
	if title, ok := rnTabTitle(raw); ok {
		label = title
	}
	enabled := raw.Enabled == nil || *raw.Enabled
	n := &sightmap.ComponentNode{
		Id:            strconv.Itoa(len(t.Nodes) + 1),
		Role:          c.tag,
		Name:          label,
		Value:         string(raw.Value),
		Element:       &sightmap.Element{Tag: c.tag, Attrs: map[string]string{}},
		Bounds:        &sightmap.Bounds{X: round(raw.Frame.X), Y: round(raw.Frame.Y), Width: round(raw.Frame.W), Height: round(raw.Frame.H)},
		IsVisible:     intersects(raw.Frame, t.Screen),
		IsInteractive: isInteractive(raw, c.tag),
		IsIgnored:     isWrapper(raw, c.tag),
		NthChild:      nth,
		Properties:    map[string]string{},
		State:         state(raw, c.tag, enabled),
	}
	n.InViewport = n.IsVisible
	attrs := n.Element.Attrs
	if raw.Identifier != "" {
		attrs["identifier"] = raw.Identifier
		// Web and React Native corpora select on `testid`; in the simulator the
		// same testID arrives as the accessibility identifier.
		attrs["testid"] = raw.Identifier
		if cssIdent.MatchString(raw.Identifier) {
			n.Element.Id = raw.Identifier
		}
	}
	if label != "" {
		attrs["text"] = label
	}
	if raw.Value != "" {
		attrs["value"] = string(raw.Value)
	}
	if !enabled {
		attrs["disabled"] = ""
	}
	t.Nodes = append(t.Nodes, n)
	if parent != nil {
		t.parent[n] = parent
	}
	t.traits[n] = raw.Traits
	for i, ch := range c.children {
		n.Children = append(n.Children, t.build(ch, n, i+1))
	}
	n.Text = label
	if n.Text == "" {
		var parts []string
		for _, ch := range n.Children {
			if ch.Text != "" {
				parts = append(parts, ch.Text)
			}
		}
		n.Text = truncateRunes(strings.Join(parts, " "), maxText)
	}
	return n
}

func state(raw *RawElement, tag string, enabled bool) map[string]string {
	s := map[string]string{
		"disabled": strconv.FormatBool(!enabled),
		"selected": strconv.FormatBool(slices.Contains(raw.Traits, "Selected")),
	}
	if tag == "switch" || tag == "checkbox" {
		switch raw.Value {
		case "1":
			s["checked"] = "true"
		case "0":
			s["checked"] = "false"
		}
	}
	return s
}

func round(f float64) int { return int(math.Round(f)) }

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Parent returns n's parent, or nil for the root.
func (t *Tree) Parent(n *sightmap.ComponentNode) *sightmap.ComponentNode { return t.parent[n] }

// Traits returns n's accessibility traits (e.g. Button, Selected).
func (t *Tree) Traits(n *sightmap.ComponentNode) []string { return t.traits[n] }

// Node returns the node with snapshot id, or nil.
func (t *Tree) Node(id string) *sightmap.ComponentNode {
	i, err := strconv.Atoi(id)
	if err != nil || i < 1 || i > len(t.Nodes) {
		return nil
	}
	return t.Nodes[i-1]
}

// IsAncestor reports whether a is a proper ancestor of n.
func (t *Tree) IsAncestor(a, n *sightmap.ComponentNode) bool {
	for p := t.parent[n]; p != nil; p = t.parent[p] {
		if p == a {
			return true
		}
	}
	return false
}

func (t *Tree) hasAncestor(n *sightmap.ComponentNode, pred func(*sightmap.ComponentNode) bool) bool {
	for p := t.parent[n]; p != nil; p = t.parent[p] {
		if pred(p) {
			return true
		}
	}
	return false
}

// Identifier returns n's accessibility identifier.
func Identifier(n *sightmap.ComponentNode) string { return n.Element.Attrs["identifier"] }

// Enabled reports whether n is enabled.
func Enabled(n *sightmap.ComponentNode) bool { return n.State["disabled"] != "true" }

// Center returns the center of n's frame in points.
func Center(n *sightmap.ComponentNode) (x, y int) {
	b := n.Bounds
	return b.X + b.Width/2, b.Y + b.Height/2
}

// OnScreen reports whether n's center is on the screen.
func (t *Tree) OnScreen(n *sightmap.ComponentNode) bool {
	b := n.Bounds
	x, y := Center(n)
	return b.Width > 0 && b.Height > 0 && x >= 0 && y >= 0 && float64(x) <= t.Screen.W && float64(y) <= t.Screen.H
}
