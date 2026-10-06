package sightmap

import (
	"slices"
	"strings"
)

// emptyElement stands in for a node with no Element, so matching a bare node
// does not allocate. Matching only reads it.
var emptyElement Element

// MatchesNode reports whether a tree node satisfies rule, including the
// relational pseudo-classes that need the node's subtree (:has()) or its
// ancestors (a :not() whose argument carries a combinator). It is the
// single-node entry point: the node is treated as the root of its own chain, so
// an ancestor-constrained :not() argument finds no ancestor and therefore does
// not exclude. Callers that hold the node's ancestor chain (the NFA matcher)
// should use MatchesNodeChain so those :not() arguments are evaluated fully.
// Tree-walking callers (sel-check, lint, coverage) use this.
func MatchesNode(node *ComponentNode, rule *SelectorPart) bool {
	return MatchesNodeChain([]*ComponentNode{node}, rule)
}

// MatchesNodeChain reports whether the LAST node of chain (the subject) satisfies
// rule. chain is root-first (chain[0] is the outermost ancestor, chain[len-1] is
// the subject); chain[:len-1] is the subject's ancestor path, used to evaluate a
// :not() argument that carries a combinator (an ancestor constraint on the
// subject). :has() is evaluated against the subject's own subtree. A nil rule
// matches everything.
func MatchesNodeChain(chain []*ComponentNode, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	if len(chain) == 0 {
		return matchesNodeChain([]*ComponentNode{{}}, 0, rule)
	}
	return matchesNodeChain(chain, len(chain)-1, rule)
}

// matchesNodeChain reports whether chain[i] satisfies rule, using chain[:i] as
// its ancestors (for combinator-bearing :not() arguments) and chain[i]'s subtree
// (for :has()). It is the recursion core shared by MatchesNodeChain, :is()
// alternatives, and :not() argument subjects, so every nested relational pseudo
// is honored with the correct tree context.
func matchesNodeChain(chain []*ComponentNode, i int, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	node := chain[i]
	el := node.Element
	if el == nil {
		el = &emptyElement
	}
	// Flat identity: tag, id, classes, attributes.
	if !matchesIdentity(chain[:i+1], el, rule) {
		return false
	}
	// :is() / :where() — the subject must match at least one alternative.
	// Evaluated node-aware so an alternative may itself carry :has()/:not().
	if len(rule.Is) > 0 {
		anyMatch := false
		for _, alt := range rule.Is {
			if matchesNodeChain(chain, i, alt) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}
	// :not() — exclude if the subject matches ANY argument (as that argument's
	// subject). Multiple :not() and comma-lists both flatten into rule.Not.
	for n := range rule.Not {
		arg := rule.Not[n]
		if complexSubjectMatch(chain, i, arg.Parts, arg.Combinators, len(arg.Parts)-1) {
			return false
		}
	}
	// :has() — each entry is AND-ed; within an entry, alternatives are OR-ed.
	for _, h := range rule.Has {
		if !hasMatches(node, h) {
			return false
		}
	}
	return true
}

// complexSubjectMatch reports whether chain[i] matches the complex selector
// parts[:pi+1] (root-first, combinators[k] precedes parts[k], combinators[0]=="")
// with parts[pi] as the subject. parts[pi] is tested against chain[i]; each
// earlier part is matched against an ancestor per its combinator (">" = direct
// parent, " " = any ancestor). Used to evaluate a :not() argument.
func complexSubjectMatch(chain []*ComponentNode, i int, parts []*SelectorPart, combs []string, pi int) bool {
	if pi < 0 || pi >= len(parts) {
		return false
	}
	if !matchesNodeChain(chain, i, parts[pi]) {
		return false
	}
	if pi == 0 {
		return true
	}
	// combs[pi] links parts[pi-1] (ancestor) to parts[pi] (this subject).
	if combs[pi] == ">" {
		return i > 0 && complexSubjectMatch(chain, i-1, parts, combs, pi-1)
	}
	for a := i - 1; a >= 0; a-- {
		if complexSubjectMatch(chain, a, parts, combs, pi-1) {
			return true
		}
	}
	return false
}

// hasMatches reports whether node's subtree satisfies the :has() selector h
// (any of its comma-separated alternatives).
func hasMatches(node *ComponentNode, h *HasSelector) bool {
	for _, alt := range h.Alternatives {
		if relMatches(node, alt.Parts, alt.Combinators, 0) {
			return true
		}
	}
	return false
}

// relMatches reports whether the relative chain parts[idx:] can be satisfied
// within anchor's subtree. combs[idx] links anchor to parts[idx]: ">" means
// parts[idx] must match a direct child of anchor; " " means any descendant.
// MatchesNode is used for each candidate so nested :has() works.
func relMatches(anchor *ComponentNode, parts []*SelectorPart, combs []string, idx int) bool {
	direct := combs[idx] == ">"
	var search func(n *ComponentNode) bool
	search = func(n *ComponentNode) bool {
		for _, child := range n.Children {
			if MatchesNode(child, parts[idx]) {
				if idx+1 == len(parts) {
					return true
				}
				if relMatches(child, parts, combs, idx+1) {
					return true
				}
			}
			if !direct && search(child) {
				return true
			}
		}
		return false
	}
	return search(anchor)
}

// Matches reports whether el satisfies rule using only the element's own
// identity (no tree). It checks tag, id, classes, attribute operators, :is()/
// :where(), and the tree-free part of :not(). It CANNOT evaluate :has() (needs a
// subtree), a :not() argument carrying a combinator (needs ancestors), or a
// :not(:has()) (needs a subtree): those are skipped here, so this is a best-
// effort element-only check — use MatchesNode / MatchesNodeChain for full,
// tree-aware matching. A nil rule matches everything. el is the observed
// element's identity (the subject).
func Matches(el *Element, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	if !matchesIdentity(nil, el, rule) {
		return false
	}

	// :is() / :where() — node must match at least one alternative.
	if len(rule.Is) > 0 {
		anyMatch := false
		for _, alt := range rule.Is {
			if Matches(el, alt) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}

	// :not() — element-only: an argument that is a single compound with no :has()
	// can be evaluated against el alone. Complex (ancestor-constrained) or
	// :has()-bearing arguments need tree context and are skipped here.
	for n := range rule.Not {
		arg := rule.Not[n]
		if len(arg.Parts) == 1 && len(arg.Parts[0].Has) == 0 && Matches(el, arg.Parts[0]) {
			return false
		}
	}

	return true
}

// matchesIdentity checks the identity of el against rule: tag, id, classes,
// and attribute operators. It ignores the logical/relational pseudos
// (:is/:not/:has), which the callers layer on with the appropriate context.
// ancestors is el's root-first chain ending at el's own node, or nil when only
// el is known; it is consulted only to tell HTML elements from SVG and MathML
// ones for the attributes whose values HTML compares case-insensitively.
func matchesIdentity(ancestors []*ComponentNode, el *Element, rule *SelectorPart) bool {
	// Type selectors compare ASCII case-insensitively, as CSS does for HTML
	// (and as synthetic mobile tags rely on). Unicode folding would also equate
	// tags CSS keeps apart, such as "a-s" and "a-ſ".
	if rule.Tag != "" && !asciiEqualFold(el.Tag, rule.Tag) {
		return false
	}

	// ID match.
	if rule.Id != "" && el.Id != rule.Id {
		return false
	}

	// Class match: every class in rule.Classes must appear in el.Classes. A
	// linear scan: elements carry a handful of classes, and this runs once per
	// (node, candidate rule), so building a set here dominated matching.
	for _, cls := range rule.Classes {
		if !slices.Contains(el.Classes, cls) {
			return false
		}
	}

	// Attribute match.
	for key, ruleVal := range rule.Attrs {
		op := "="
		if rule.AttrOps != nil {
			if o, ok := rule.AttrOps[key]; ok {
				op = o
			}
		}
		if !attrTestHolds(ancestors, el, key, op, ruleVal) {
			return false
		}
	}
	for _, at := range rule.RepeatAttrs {
		if !attrTestHolds(ancestors, el, at.Key, at.Op, at.Value) {
			return false
		}
	}

	return true
}

// attrTestHolds reports whether el satisfies one attribute test. ancestors is
// as for matchesIdentity.
func attrTestHolds(ancestors []*ComponentNode, el *Element, key, op, ruleVal string) bool {
	// Every operator, presence-only included, requires the attribute.
	if !el.HasAttr(key) {
		return false
	}

	if key == "class" && len(el.Classes) > 1 {
		if _, inAttrs := el.Attrs["class"]; !inAttrs {
			if matched, ok := classAttrMatches(op, el.Classes, ruleVal); ok {
				return matched
			}
		}
	}

	nodeVal, _ := el.Attr(key)
	if attrMatches(op, nodeVal, ruleVal) {
		return true
	}
	// HTML compares some attribute values ASCII case-insensitively on HTML
	// elements, so input[type=password] matches type="PASSWORD". Checked only
	// after a case-sensitive miss, so the common path never walks ancestors.
	return AttrValueCaseInsensitive(key) &&
		attrMatches(op, asciiLower(nodeVal), asciiLower(ruleVal)) &&
		!isForeign(ancestors, el)
}

// Attr resolves an attribute value on an observed Element the way selector
// matching sees it. id and class live in dedicated fields (Id, Classes) — not always in
// Attrs — so attribute selectors like [id^="issue_"] or [class*="card"] must see
// them there to match offline the way the browser matches them live. Attrs is
// consulted first (it wins when populated); id/class then fall back to their
// dedicated fields. All other attributes come straight from Attrs.
func (el *Element) Attr(key string) (string, bool) {
	if v, ok := el.Attrs[key]; ok {
		return v, true
	}
	// Selector keys are lowercased at parse time (HTML attribute names are
	// ASCII case-insensitive), but captures keep the DOM's spelling, which for
	// SVG is camelCase (viewBox, preserveAspectRatio). Retry under that
	// spelling so [viewBox="0 0 24 24"] matches offline as it does in a browser.
	if len(el.Attrs) > 0 {
		if dom := foreignAttrName(key); dom != "" {
			if v, ok := el.Attrs[dom]; ok {
				return v, true
			}
		}
	}
	switch key {
	case "id":
		if el.Id != "" {
			return el.Id, true
		}
	case "class":
		if len(el.Classes) == 1 {
			return el.Classes[0], true
		}
		if len(el.Classes) > 1 {
			return strings.Join(el.Classes, " "), true
		}
	}
	return "", false
}

// HasAttr reports whether Attr would find key, without building its value.
func (el *Element) HasAttr(key string) bool {
	if _, ok := el.Attrs[key]; ok {
		return true
	}
	if len(el.Attrs) > 0 {
		if dom := foreignAttrName(key); dom != "" {
			if _, ok := el.Attrs[dom]; ok {
				return true
			}
		}
	}
	switch key {
	case "id":
		return el.Id != ""
	case "class":
		return len(el.Classes) > 0
	}
	return false
}

// classAttrMatches evaluates a class attribute selector against classes as
// attrMatches would against strings.Join(classes, " "), without the join: it
// runs once per (element, candidate rule) and the join dominated matching
// against corpora with [class*=...] selectors. When ruleVal has no space, a
// match in the joined string cannot cross the separator, so each operator
// reduces to a per-class check. ok is false when that reduction does not hold
// (ruleVal empty or containing a space); the caller must join instead.
func classAttrMatches(op string, classes []string, ruleVal string) (matched, ok bool) {
	if ruleVal == "" || strings.IndexByte(ruleVal, ' ') >= 0 {
		return false, false
	}
	switch op {
	case "=":
		// The joined value of two or more classes contains a space; ruleVal does not.
		return len(classes) == 1 && classes[0] == ruleVal, true
	case "[]":
		return true, true
	case "^=":
		return strings.HasPrefix(classes[0], ruleVal), true
	case "$=":
		return strings.HasSuffix(classes[len(classes)-1], ruleVal), true
	case "*=", "~=":
		for _, c := range classes {
			if attrMatches(op, c, ruleVal) {
				return true, true
			}
		}
		return false, true
	case "|=":
		return (len(classes) == 1 && classes[0] == ruleVal) || strings.HasPrefix(classes[0], ruleVal+"-"), true
	default:
		return false, true
	}
}

// attrMatches returns whether a node attribute value satisfies the operator
// against the rule value.
func attrMatches(op, nodeVal, ruleVal string) bool {
	switch op {
	case "=":
		return nodeVal == ruleVal
	case "[]":
		// Presence-only — attribute exists (we already checked above).
		return true
	case "^=":
		return ruleVal != "" && strings.HasPrefix(nodeVal, ruleVal)
	case "$=":
		return ruleVal != "" && strings.HasSuffix(nodeVal, ruleVal)
	case "*=":
		return ruleVal != "" && strings.Contains(nodeVal, ruleVal)
	case "~=":
		// Whitespace-separated list includes ruleVal exactly. CSS: a value that
		// is empty or contains whitespace never matches.
		if ruleVal == "" || strings.ContainsAny(ruleVal, " \t\r\n\f") {
			return false
		}
		return includesWord(nodeVal, ruleVal)
	case "|=":
		// Equals ruleVal or starts with "ruleVal-".
		return nodeVal == ruleVal || strings.HasPrefix(nodeVal, ruleVal+"-")
	default:
		return false
	}
}

// includesWord returns true if s is a whitespace-separated list containing word.
func includesWord(s, word string) bool {
	for s != "" {
		i := strings.IndexAny(s, " \t\r\n\f")
		var token string
		if i < 0 {
			token = s
			s = ""
		} else {
			token = s[:i]
			s = s[i+1:]
		}
		if token == word {
			return true
		}
	}
	return false
}

// foreignAttrName returns the mixed-case spelling the HTML parser gives key on
// SVG and MathML elements ("adjust SVG attributes" and "adjust MathML
// attributes" in the HTML spec), or "" when it has none. These are the only
// mixed-case attribute names a parsed HTML document produces. A switch rather
// than a map: most Attr calls miss, and hashing key on each miss showed up in
// BenchmarkFindAllMatches.
func foreignAttrName(key string) string {
	switch key {
	case "attributename":
		return "attributeName"
	case "attributetype":
		return "attributeType"
	case "basefrequency":
		return "baseFrequency"
	case "baseprofile":
		return "baseProfile"
	case "calcmode":
		return "calcMode"
	case "clippathunits":
		return "clipPathUnits"
	case "definitionurl":
		return "definitionURL"
	case "diffuseconstant":
		return "diffuseConstant"
	case "edgemode":
		return "edgeMode"
	case "filterunits":
		return "filterUnits"
	case "glyphref":
		return "glyphRef"
	case "gradienttransform":
		return "gradientTransform"
	case "gradientunits":
		return "gradientUnits"
	case "kernelmatrix":
		return "kernelMatrix"
	case "kernelunitlength":
		return "kernelUnitLength"
	case "keypoints":
		return "keyPoints"
	case "keysplines":
		return "keySplines"
	case "keytimes":
		return "keyTimes"
	case "lengthadjust":
		return "lengthAdjust"
	case "limitingconeangle":
		return "limitingConeAngle"
	case "markerheight":
		return "markerHeight"
	case "markerunits":
		return "markerUnits"
	case "markerwidth":
		return "markerWidth"
	case "maskcontentunits":
		return "maskContentUnits"
	case "maskunits":
		return "maskUnits"
	case "numoctaves":
		return "numOctaves"
	case "pathlength":
		return "pathLength"
	case "patterncontentunits":
		return "patternContentUnits"
	case "patterntransform":
		return "patternTransform"
	case "patternunits":
		return "patternUnits"
	case "pointsatx":
		return "pointsAtX"
	case "pointsaty":
		return "pointsAtY"
	case "pointsatz":
		return "pointsAtZ"
	case "preservealpha":
		return "preserveAlpha"
	case "preserveaspectratio":
		return "preserveAspectRatio"
	case "primitiveunits":
		return "primitiveUnits"
	case "refx":
		return "refX"
	case "refy":
		return "refY"
	case "repeatcount":
		return "repeatCount"
	case "repeatdur":
		return "repeatDur"
	case "requiredextensions":
		return "requiredExtensions"
	case "requiredfeatures":
		return "requiredFeatures"
	case "specularconstant":
		return "specularConstant"
	case "specularexponent":
		return "specularExponent"
	case "spreadmethod":
		return "spreadMethod"
	case "startoffset":
		return "startOffset"
	case "stddeviation":
		return "stdDeviation"
	case "stitchtiles":
		return "stitchTiles"
	case "surfacescale":
		return "surfaceScale"
	case "systemlanguage":
		return "systemLanguage"
	case "tablevalues":
		return "tableValues"
	case "targetx":
		return "targetX"
	case "targety":
		return "targetY"
	case "textlength":
		return "textLength"
	case "viewbox":
		return "viewBox"
	case "viewtarget":
		return "viewTarget"
	case "xchannelselector":
		return "xChannelSelector"
	case "ychannelselector":
		return "yChannelSelector"
	case "zoomandpan":
		return "zoomAndPan"
	}
	return ""
}

// legacyCaseInsensitiveAttrs are the attributes whose values HTML compares
// ASCII case-insensitively in selectors, on HTML elements only. See
// https://html.spec.whatwg.org/multipage/semantics-other.html#case-sensitivity-of-selectors
var legacyCaseInsensitiveAttrs = map[string]bool{
	"accept": true, "accept-charset": true, "align": true, "alink": true, "axis": true,
	"bgcolor": true, "charset": true, "checked": true, "clear": true, "codetype": true,
	"color": true, "compact": true, "declare": true, "defer": true, "dir": true,
	"direction": true, "disabled": true, "enctype": true, "face": true, "frame": true,
	"hreflang": true, "http-equiv": true, "lang": true, "language": true, "link": true,
	"media": true, "method": true, "multiple": true, "nohref": true, "noresize": true,
	"noshade": true, "nowrap": true, "readonly": true, "rel": true, "rev": true,
	"rules": true, "scope": true, "scrolling": true, "selected": true, "shape": true,
	"target": true, "text": true, "type": true, "valign": true, "valuetype": true,
	"vlink": true,
}

// AttrValueCaseInsensitive reports whether HTML compares the value of the named
// attribute (lowercase) ASCII case-insensitively in selectors on HTML elements.
func AttrValueCaseInsensitive(name string) bool {
	return legacyCaseInsensitiveAttrs[name]
}

// isForeign reports whether el is an SVG or MathML element: the nearest svg,
// math or foreignObject among el and its ancestors is svg or math. el itself
// being foreignObject counts as SVG; its content is HTML. ancestors is el's
// root-first chain ending at el's own node; nil means only el is known.
func isForeign(ancestors []*ComponentNode, el *Element) bool {
	switch asciiLower(el.Tag) {
	case "svg", "math", "foreignobject":
		return true
	}
	for i := len(ancestors) - 2; i >= 0; i-- {
		if ancestors[i].Element == nil {
			continue
		}
		switch asciiLower(ancestors[i].Element.Tag) {
		case "foreignobject":
			return false
		case "svg", "math":
			return true
		}
	}
	return false
}

// asciiEqualFold reports whether a and b are equal under ASCII case folding.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// asciiLower lowercases ASCII letters, returning s itself when it has none.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if 'A' <= b[j] && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}
