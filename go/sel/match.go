package sel

import (
	"strings"

	"github.com/sightmap/sightmap/go/comps"
)

// MatchesNode reports whether a tree node satisfies rule, including the
// relational :has() pseudo-class (which needs the node's subtree). It first
// applies the flat checks via Matches, then verifies every :has() entry against
// node's descendants. Tree-walking callers (the rule matcher, sel-check, lint)
// should use this instead of Matches so :has() is honored.
func MatchesNode(node *comps.ComponentNode, rule *comps.SelectorPart) bool {
	if rule == nil {
		return true
	}
	nodeSel := node.Selector
	if nodeSel == nil {
		nodeSel = &comps.SelectorPart{}
	}
	if !Matches(nodeSel, rule) {
		return false
	}
	// :has() — each entry is AND-ed; within an entry, alternatives are OR-ed.
	for _, h := range rule.Has {
		if !hasMatches(node, h) {
			return false
		}
	}
	return true
}

// hasMatches reports whether node's subtree satisfies the :has() selector h
// (any of its comma-separated alternatives).
func hasMatches(node *comps.ComponentNode, h *comps.HasSelector) bool {
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
func relMatches(anchor *comps.ComponentNode, parts []*comps.SelectorPart, combs []string, idx int) bool {
	direct := combs[idx] == ">"
	var search func(n *comps.ComponentNode) bool
	search = func(n *comps.ComponentNode) bool {
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

// Matches reports whether node satisfies rule. It checks tag, id, classes,
// attribute operators, :not(), and :is()/:where(). It does NOT evaluate :has()
// (which needs tree context — use MatchesNode for that). A nil rule matches
// everything.
func Matches(node, rule *comps.SelectorPart) bool {
	if rule == nil {
		return true
	}

	// Tag match (case-insensitive for HTML; case-sensitive for synthetic mobile).
	if rule.Tag != "" && !strings.EqualFold(node.Tag, rule.Tag) {
		return false
	}

	// ID match.
	if rule.Id != "" && node.Id != rule.Id {
		return false
	}

	// Class match: every class in rule.Classes must appear in node.Classes.
	if len(rule.Classes) > 0 {
		nodeClasses := sliceToSet(node.Classes)
		for _, cls := range rule.Classes {
			if !nodeClasses[cls] {
				return false
			}
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

		nodeVal, present := node.Attrs[key]
		if !present {
			// Attribute not present on node — only "[]" (presence-only) would
			// logically not care, but absence means presence check fails too.
			return false
		}

		if !attrMatches(op, nodeVal, ruleVal) {
			return false
		}
	}

	// :not() check.
	if rule.Not != nil && Matches(node, rule.Not) {
		return false
	}

	// :is() / :where() — node must match at least one alternative.
	if len(rule.Is) > 0 {
		anyMatch := false
		for _, alt := range rule.Is {
			if Matches(node, alt) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}

	return true
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
		// Whitespace-separated list includes ruleVal exactly.
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

// sliceToSet converts a string slice to a presence map.
func sliceToSet(ss []string) map[string]bool {
	if len(ss) == 0 {
		return nil
	}
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
