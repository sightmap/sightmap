package device

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// SwiftUI's view debug graph (makeViewDebugData) is private and noisy: a row
// of three Texts is ~180 nodes, almost all SwiftUI wrappers whose names change
// between releases. Rather than filter that noise out, the merge picks out two
// things it understands and ignores the rest:
//
//   - app views: a node, or a field in a node's value, whose type comes from a
//     module that is not Apple's (a NavigationLink's `label: ShopRow`);
//   - Text leaves, with their own string or, for Text(value, format:), the
//     unformatted value.
//
// Each lands under the out-of-process element it belongs to, found by the
// accessibility identifier on the nearest enclosing
// AccessibilityAttachmentModifier, else by frame.

// graphNode is one node of a host's debug graph.
type graphNode struct {
	Properties []struct {
		ID        int        `json:"id"`
		Attribute *graphAttr `json:"attribute"`
	} `json:"properties"`
	Children []*graphNode `json:"children"`
}

type graphAttr struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Readable string          `json:"readableType"`
	Value    json.RawMessage `json:"value"`
	Sub      []*graphAttr    `json:"subattributes"`
}

// Property ids in the debug data.
const (
	propType     = 0
	propValue    = 1
	propPosition = 3
	propSize     = 4
)

func (n *graphNode) prop(id int) *graphAttr {
	for _, p := range n.Properties {
		if p.ID == id {
			return p.Attribute
		}
	}
	return nil
}

func (a *graphAttr) child(name string) *graphAttr {
	if a == nil {
		return nil
	}
	for _, s := range a.Sub {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// find returns the first attribute, depth first and at most depth deep, that
// pred accepts.
func (a *graphAttr) find(depth int, pred func(*graphAttr) bool) *graphAttr {
	if a == nil || depth < 0 {
		return nil
	}
	if pred(a) {
		return a
	}
	for _, s := range a.Sub {
		if f := s.find(depth-1, pred); f != nil {
			return f
		}
	}
	return nil
}

var appleModules = map[string]bool{
	"SwiftUI": true, "SwiftUICore": true, "Swift": true, "Foundation": true, "UIKit": true, "CoreGraphics": true,
	"CoreFoundation": true, "Combine": true, "Observation": true, "_Concurrency": true, "ObjectiveC": true,
	"AttributeGraph": true, "CoreText": true, "QuartzCore": true, "MapKit": true, "Charts": true, "PhotosUI": true,
	"AVKit": true, "WebKit": true, "StoreKit": true, "__C": true,
}

// appType returns the unqualified, non-generic name of a type from a
// non-Apple module, or "".
func appType(qualified string) string {
	mod, rest, ok := strings.Cut(qualified, ".")
	if !ok || appleModules[mod] || strings.HasPrefix(mod, "(") || strings.ContainsAny(mod, "<(") {
		return ""
	}
	if i := strings.IndexByte(rest, '<'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// probeView is an app view found in the graph, with its stored properties
// flattened to scalars ("item.title" → "Socks").
type probeView struct {
	Name   string
	Fields [][2]string
}

func (v *probeView) fieldFor(value string) string {
	for _, f := range v.Fields {
		if f[1] == value {
			return f[0]
		}
	}
	return ""
}

const maxViewFields = 32

func flattenFields(a *graphAttr, prefix string, out *[][2]string) {
	if len(*out) >= maxViewFields {
		return
	}
	path := a.Name
	if prefix != "" {
		path = prefix + "." + a.Name
	}
	if len(a.Value) > 0 {
		flattenJSON(a.Value, path, out)
		return
	}
	for _, s := range a.Sub {
		flattenFields(s, path, out)
	}
}

func flattenJSON(raw json.RawMessage, path string, out *[][2]string) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	var walk func(any, string)
	walk = func(v any, path string) {
		if len(*out) >= maxViewFields {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k], path+"."+k)
			}
		case string:
			*out = append(*out, [2]string{path, truncateRunes(t, maxText)})
		case float64:
			*out = append(*out, [2]string{path, formatNumber(t)})
		case bool:
			*out = append(*out, [2]string{path, strconv.FormatBool(t)})
		}
	}
	walk(v, path)
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// probeItem is one thing the merge places: an app view, or a Text inside the
// nearest app view.
type probeItem struct {
	Anchor string // accessibility identifier of the enclosing modifier
	View   *probeView
	Text   string // shown text, or the unformatted value of a formatted Text
	Value  string // the unformatted value, for a formatted Text
	Format string // e.g. "currency USD"
	Frame  Frame  // screen points; zero when the graph has no geometry for it
}

type graphWalk struct {
	origin [2]float64
	items  []*probeItem
}

func (w *graphWalk) walk(n *graphNode, anchor string, view *probeView, frame Frame) {
	if pos, size := n.prop(propPosition), n.prop(propSize); pos != nil && size != nil {
		var p, s [2]float64
		if json.Unmarshal(pos.Value, &p) == nil && json.Unmarshal(size.Value, &s) == nil {
			frame = Frame{X: w.origin[0] + p[0], Y: w.origin[1] + p[1], W: s[0], H: s[1]}
		}
	}
	typ, val := n.prop(propType), n.prop(propValue)
	readable := ""
	if typ != nil {
		readable = typ.Readable
	}
	if id := attachmentIdentifier(readable, val); id != "" {
		anchor = id
	}
	if v := w.appView(typ, val, view); v != nil {
		view = v
		w.items = append(w.items, &probeItem{Anchor: anchor, View: v, Frame: frame})
	}
	if readable == "Text" {
		if it := textItem(val); it != nil {
			it.Anchor, it.View, it.Frame = anchor, view, frame
			w.items = append(w.items, it)
		}
	}
	for _, c := range n.Children {
		w.walk(c, anchor, view, frame)
	}
}

func attachmentIdentifier(readable string, val *graphAttr) string {
	if !strings.HasPrefix(readable, "AccessibilityAttachmentModifier") {
		return ""
	}
	id := val.find(8, func(a *graphAttr) bool {
		return a.Name == "identifier" && strings.Contains(a.Type, "AccessibilityIdentifierStorage")
	})
	var s string
	if raw := id.child("rawValue"); raw != nil && json.Unmarshal(raw.Value, &s) == nil {
		return s
	}
	return ""
}

// appView returns the app view a node is, or carries as a field of its value
// (a NavigationLink's or Button's label), unless it is the view already in
// effect: SwiftUI repeats a link's label down a chain of wrapper nodes.
func (w *graphWalk) appView(typ, val *graphAttr, current *probeView) *probeView {
	var name string
	var src *graphAttr
	if typ != nil {
		if name = appType(typ.Type); name != "" {
			src = val
		}
	}
	if name == "" && val != nil {
		for _, s := range val.Sub {
			if n := appType(s.Type); n != "" {
				name, src = n, s
				break
			}
		}
	}
	if name == "" {
		return nil
	}
	v := &probeView{Name: name}
	if src != nil {
		for _, s := range src.Sub {
			flattenFields(s, "", &v.Fields)
		}
	}
	if current != nil && current.Name == v.Name && fmt.Sprint(current.Fields) == fmt.Sprint(v.Fields) {
		return nil
	}
	return v
}

func textItem(val *graphAttr) *probeItem {
	storage := val.child("storage")
	if storage == nil {
		return nil
	}
	if verbatim := storage.child("verbatim"); verbatim != nil {
		var s string
		if json.Unmarshal(verbatim.Value, &s) == nil {
			return &probeItem{Text: s}
		}
	}
	// LocalizedStringKey and formatted storage nest a level or two deeper.
	if key := storage.find(4, func(a *graphAttr) bool { return a.Name == "key" && len(a.Value) > 0 }); key != nil {
		var s string
		if json.Unmarshal(key.Value, &s) == nil {
			return &probeItem{Text: s}
		}
	}
	if input := storage.find(4, func(a *graphAttr) bool { return a.Name == "input" && len(a.Value) > 0 }); input != nil {
		var out [][2]string
		flattenJSON(input.Value, "", &out)
		if len(out) == 1 {
			it := &probeItem{Text: out[0][1], Value: out[0][1]}
			if f := storage.find(4, func(a *graphAttr) bool { return a.Name == "format" }); f != nil {
				it.Format = describeFormat(f)
			}
			return it
		}
	}
	return nil
}

// describeFormat names a FormatStyle briefly: "currency USD", "number", "date".
func describeFormat(f *graphAttr) string {
	t := f.Readable
	var v map[string]any
	json.Unmarshal(f.Value, &v)
	switch {
	case strings.Contains(t, "Currency"):
		if code, ok := v["currencyCode"].(string); ok {
			return "currency " + code
		}
		return "currency"
	case strings.Contains(t, "Percent"):
		return "percent"
	case strings.Contains(t, "Date"):
		return "date"
	case strings.Contains(t, "FloatingPoint"), strings.Contains(t, "Integer"), strings.Contains(t, "Decimal"):
		return "number"
	}
	return ""
}

// ProbeItems decodes each host's graph and returns what the merge places, in
// screen order within each host.
func ProbeItems(dump *ProbeDump) ([]*probeItem, error) {
	var all []*probeItem
	for _, h := range dump.Hosts {
		var roots []*graphNode
		if err := json.Unmarshal(h.Graph, &roots); err != nil {
			return nil, fmt.Errorf("host %s: %w", h.Info.Class, err)
		}
		w := &graphWalk{origin: [2]float64{h.Info.F[0], h.Info.F[1]}}
		for _, r := range roots {
			w.walk(r, "", nil, Frame{X: h.Info.F[0], Y: h.Info.F[1], W: h.Info.F[2], H: h.Info.F[3]})
		}
		all = append(all, w.items...)
	}
	return all, nil
}

// MergeOptions tunes MergeProbe.
type MergeOptions struct {
	// Fields copies an app view's stored fields onto its view node, redacted.
	Fields bool
}

// MergeProbe adds the probe's app views and Texts under the elements of d they
// belong to. An element gets children only when its own label already says
// what they say (SwiftUI joined them into it); elements whose texts the
// out-of-process tree already has separately are left alone.
func MergeProbe(d *Describe, items []*probeItem, opts MergeOptions) {
	if d.Root == nil {
		return
	}
	byID := map[string][]*RawElement{}
	var index func(e *RawElement)
	index = func(e *RawElement) {
		if e.Identifier != "" {
			byID[e.Identifier] = append(byID[e.Identifier], e)
		}
		for _, c := range e.Children {
			index(c)
		}
	}
	index(d.Root)

	type group struct {
		target *RawElement
		views  []*probeView
		items  map[*probeView][]*probeItem
		frames map[*probeView]Frame
	}
	groups := map[*RawElement]*group{}
	var order []*RawElement
	for _, it := range items {
		target := anchorElement(byID[it.Anchor], it.Frame)
		if target == nil {
			target = deepestCombining(d.Root, it)
		}
		if target == nil || tagFor(target) == "securetextfield" {
			continue
		}
		g := groups[target]
		if g == nil {
			g = &group{target: target, items: map[*probeView][]*probeItem{}, frames: map[*probeView]Frame{}}
			groups[target] = g
			order = append(order, target)
		}
		if it.Text == "" {
			if _, seen := g.items[it.View]; !seen {
				g.views = append(g.views, it.View)
				g.items[it.View] = nil
				g.frames[it.View] = it.Frame
			}
			continue
		}
		if it.Text == target.Label || !strings.Contains(target.Label, it.Text) && it.Value == "" {
			continue
		}
		if hasChildLabel(target, it.Text) {
			continue
		}
		if _, seen := g.items[it.View]; !seen {
			g.views = append(g.views, it.View)
			g.frames[it.View] = it.Frame
		}
		g.items[it.View] = append(g.items[it.View], it)
	}
	for _, target := range order {
		g := groups[target]
		for _, v := range g.views {
			texts := g.items[v]
			if len(texts) == 0 {
				continue
			}
			sort.SliceStable(texts, func(i, j int) bool {
				a, b := texts[i].Frame, texts[j].Frame
				if math.Abs(a.Y-b.Y) > 1 {
					return a.Y < b.Y
				}
				return a.X < b.X
			})
			parent := target
			if v != nil {
				el := &RawElement{Type: "view", Frame: target.Frame, Attrs: map[string]string{"type": v.Name}, Probe: true}
				if opts.Fields {
					for _, f := range v.Fields {
						if f[0] != "type" {
							el.Attrs[f[0]] = redactField(f[0], f[1])
						}
					}
				}
				target.Children = append(target.Children, el)
				parent = el
			}
			for _, it := range texts {
				el := &RawElement{Type: "StaticText", Label: it.Text, Value: Value(it.Value), Frame: it.Frame, Attrs: map[string]string{}, Probe: true}
				if el.Frame.W == 0 {
					el.Frame = parent.Frame
				}
				if it.Format != "" {
					el.Attrs["format"] = it.Format
				}
				if v != nil {
					if f := v.fieldFor(it.Text); f != "" {
						el.Attrs["field"] = f
						if redactField(f, it.Text) == Redacted {
							el.Label, el.Value = Redacted, ""
						}
					}
				}
				parent.Children = append(parent.Children, el)
			}
		}
	}
}

func anchorElement(els []*RawElement, f Frame) *RawElement {
	if len(els) == 1 {
		return els[0]
	}
	for _, e := range els {
		if contains(e.Frame, f) {
			return e
		}
	}
	return nil
}

func contains(outer, inner Frame) bool {
	cx, cy := inner.X+inner.W/2, inner.Y+inner.H/2
	return inner.W > 0 && cx >= outer.X && cx <= outer.X+outer.W && cy >= outer.Y && cy <= outer.Y+outer.H
}

// deepestCombining finds, for an item with no identifier to go by, the deepest
// childless element whose frame holds it and whose label contains its text.
func deepestCombining(e *RawElement, it *probeItem) *RawElement {
	if it.Text == "" || !contains(e.Frame, it.Frame) {
		return nil
	}
	for _, c := range e.Children {
		if f := deepestCombining(c, it); f != nil {
			return f
		}
	}
	if len(e.Children) == 0 && e.Label != it.Text && strings.Contains(e.Label, it.Text) {
		return e
	}
	return nil
}

func hasChildLabel(e *RawElement, text string) bool {
	for _, c := range e.Children {
		if c.Label == text || hasChildLabel(c, text) {
			return true
		}
	}
	return false
}

// redactField applies the network log's rules to an app view's stored field:
// a name with token, password, secret or key in any segment hides the value,
// and a URL's credential query parameters are hidden.
func redactField(path, value string) string {
	for _, seg := range strings.Split(path, ".") {
		if SensitiveName(seg) {
			return Redacted
		}
	}
	if strings.Contains(value, "://") {
		return redactURL(value)
	}
	return value
}
