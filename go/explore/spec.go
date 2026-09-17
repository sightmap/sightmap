package explore

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Spec is what the loop needs beyond the goal text: how to recognise "done",
// the values it may type, an optional hint, and controls to avoid.
type Spec struct {
	DoneWhen *DoneWhen         `json:"done_when,omitempty"`
	Values   map[string]string `json:"values,omitempty"`
	Hint     string            `json:"hint,omitempty"`
	Avoid    []string          `json:"avoid,omitempty"`
}

// DoneWhen is a deterministic goal check over the current page. Every set
// field must hold; All nests further checks that must all hold.
type DoneWhen struct {
	View            string     `json:"view,omitempty"`
	URLContains     string     `json:"url_contains,omitempty"`
	TextContains    string     `json:"text_contains,omitempty"`
	Component       string     `json:"component,omitempty"`
	Prop            *PropCheck `json:"prop,omitempty"`
	HistoryContains string     `json:"history_contains,omitempty"`
	All             []DoneWhen `json:"all,omitempty"`
}

// PropCheck asserts that a visible component's extracted property contains a
// value, optionally only inside an ancestor component whose own property matches.
type PropCheck struct {
	Component string     `json:"component"`
	Name      string     `json:"name"`
	Contains  string     `json:"contains"`
	Within    *PropCheck `json:"within,omitempty"`
}

// LoadSpec reads a spec JSON file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec %s: %w", path, err)
	}
	return &s, nil
}

// ParseDoneWhen parses the flag mini-syntax: one "key=value" per expression,
// keys view, url, text, component, history, or prop=Component.name~value
// (with an optional "@Within.name~value" suffix). Several expressions are ANDed.
func ParseDoneWhen(exprs []string) (*DoneWhen, error) {
	if len(exprs) == 0 {
		return nil, nil
	}
	var parts []DoneWhen
	for _, e := range exprs {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("done-when %q: expected key=value", e)
		}
		var d DoneWhen
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "view":
			d.View = v
		case "url":
			d.URLContains = v
		case "text":
			d.TextContains = v
		case "component":
			d.Component = v
		case "history":
			d.HistoryContains = v
		case "prop":
			pc, err := parsePropCheck(v)
			if err != nil {
				return nil, fmt.Errorf("done-when %q: %w", e, err)
			}
			d.Prop = pc
		default:
			return nil, fmt.Errorf("done-when %q: unknown key %q (view, url, text, component, history, prop)", e, k)
		}
		parts = append(parts, d)
	}
	if len(parts) == 1 {
		return &parts[0], nil
	}
	return &DoneWhen{All: parts}, nil
}

// parsePropCheck parses "Component.name~value[@Within.name~value]".
func parsePropCheck(s string) (*PropCheck, error) {
	main, within, hasWithin := strings.Cut(s, "@")
	pc, err := parseOnePropCheck(main)
	if err != nil {
		return nil, err
	}
	if hasWithin {
		w, err := parseOnePropCheck(within)
		if err != nil {
			return nil, err
		}
		pc.Within = w
	}
	return pc, nil
}

func parseOnePropCheck(s string) (*PropCheck, error) {
	left, contains, ok := strings.Cut(s, "~")
	if !ok {
		return nil, fmt.Errorf("prop check %q: expected Component.name~value", s)
	}
	comp, name, ok := strings.Cut(left, ".")
	if !ok || comp == "" || name == "" {
		return nil, fmt.Errorf("prop check %q: expected Component.name~value", s)
	}
	return &PropCheck{Component: comp, Name: name, Contains: contains}, nil
}

// Deterministic reports whether the check has any condition at all.
func (d *DoneWhen) Deterministic() bool {
	return d != nil && (d.View != "" || d.URLContains != "" || d.TextContains != "" || d.Component != "" || d.Prop != nil || d.HistoryContains != "" || len(d.All) > 0)
}

// String renders the check for the picker's context.
func (d *DoneWhen) String() string {
	if !d.Deterministic() {
		return "judge from the page whether the goal is achieved"
	}
	var parts []string
	if d.View != "" {
		parts = append(parts, fmt.Sprintf("the page is the %q view", d.View))
	}
	if d.URLContains != "" {
		parts = append(parts, fmt.Sprintf("the URL contains %q", d.URLContains))
	}
	if d.TextContains != "" {
		parts = append(parts, fmt.Sprintf("the page shows the text %q", d.TextContains))
	}
	if d.Component != "" {
		parts = append(parts, fmt.Sprintf("a %s component is visible", d.Component))
	}
	if d.Prop != nil {
		s := fmt.Sprintf("%s.%s contains %q", d.Prop.Component, d.Prop.Name, d.Prop.Contains)
		if w := d.Prop.Within; w != nil {
			s += fmt.Sprintf(" inside %s whose %s contains %q", w.Component, w.Name, w.Contains)
		}
		parts = append(parts, s)
	}
	if d.HistoryContains != "" {
		parts = append(parts, fmt.Sprintf("an earlier step mentioned %q", d.HistoryContains))
	}
	for _, a := range d.All {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, " and ")
}

// Check evaluates the condition against a page and the step history.
func (d *DoneWhen) Check(page *Page, history []string) bool {
	if !d.Deterministic() {
		return false
	}
	for _, a := range d.All {
		if !a.Check(page, history) {
			return false
		}
	}
	if d.View != "" && page.View != d.View {
		return false
	}
	if d.URLContains != "" && !strings.Contains(page.URL, d.URLContains) {
		return false
	}
	if d.TextContains != "" && !pageHasText(page, d.TextContains) {
		return false
	}
	if d.Component != "" && !componentVisible(page, d.Component) {
		return false
	}
	if d.Prop != nil && !propHolds(page, d.Prop) {
		return false
	}
	if d.HistoryContains != "" {
		found := false
		for _, h := range history {
			if strings.Contains(h, d.HistoryContains) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func pageHasText(page *Page, want string) bool {
	w := strings.ToLower(want)
	for _, n := range page.Nodes {
		if !n.Visible {
			continue
		}
		if strings.Contains(strings.ToLower(n.Name), w) || strings.Contains(strings.ToLower(n.Text), w) || strings.Contains(strings.ToLower(n.Value), w) {
			return true
		}
		for _, v := range n.Props {
			if strings.Contains(strings.ToLower(v), w) {
				return true
			}
		}
	}
	return false
}

func componentVisible(page *Page, name string) bool {
	for _, n := range page.Nodes {
		if n.Comp == name && n.Visible {
			return true
		}
	}
	return false
}

// nodePropValue reads a property, falling back to the node's own value or
// accessible name when the corpus does not extract that property.
func nodePropValue(n *Node, name string) string {
	if v, ok := n.Props[name]; ok {
		return v
	}
	switch name {
	case "value":
		return n.Value
	case "name", "label", "text":
		return n.Name
	}
	return ""
}

func propHolds(page *Page, pc *PropCheck) bool {
	want := strings.ToLower(pc.Contains)
	for _, n := range page.Nodes {
		if n.Comp != pc.Component || !n.Visible {
			continue
		}
		if !strings.Contains(strings.ToLower(nodePropValue(n, pc.Name)), want) {
			continue
		}
		if pc.Within == nil {
			return true
		}
		w := pc.Within
		for _, a := range n.Ancestors {
			if a.Comp == w.Component && strings.Contains(strings.ToLower(nodePropValue(a, w.Name)), strings.ToLower(w.Contains)) {
				return true
			}
		}
	}
	return false
}
