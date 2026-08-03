package sightmap

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sel"
)

// LintWarning describes a style or maintainability issue that doesn't prevent
// correct matching but indicates fragility or future problems.
type LintWarning struct {
	File      string
	Component string
	Selector  string
	Rule      string // identifier: "name-case", "broad-tag-selector", etc.
	Message   string
}

func (w LintWarning) String() string {
	parts := []string{}
	if w.File != "" {
		parts = append(parts, w.File)
	}
	if w.Component != "" {
		parts = append(parts, w.Component)
	}
	if w.Selector != "" {
		parts = append(parts, fmt.Sprintf("selector %q", w.Selector))
	}
	parts = append(parts, fmt.Sprintf("[%s] %s", w.Rule, w.Message))
	return strings.Join(parts, ": ")
}

// autoIdRe matches selector fragments that contain a potentially auto-generated
// #id: either ending in 4+ decimal digits or containing a run of 5+ hex chars.
var autoIdRe = regexp.MustCompile(`#[a-zA-Z0-9-]*[0-9]{4,}|#[a-zA-Z0-9-]*[0-9a-f]{5,}`)

// Lint checks the corpus for antipatterns and style violations.
// It also runs Validate internally — ValidationErrors are returned as
// LintWarnings with Rule="validation".
func Lint(c *Corpus) []LintWarning {
	var warnings []LintWarning

	// Wrap every ValidationError as a lint warning.
	for _, ve := range Validate(c) {
		warnings = append(warnings, LintWarning{
			File:      ve.File,
			Component: ve.Component,
			Selector:  ve.Selector,
			Rule:      "validation",
			Message:   ve.Error(),
		})
	}

	// Lint global components (isGlobal=true enables the broad-tag-selector rule).
	for _, comp := range c.GlobalComponents {
		warnings = append(warnings, lintComponent(comp, true)...)
	}

	// Lint view components (isGlobal=false).
	for _, view := range c.Views {
		for _, comp := range view.Components {
			warnings = append(warnings, lintComponent(comp, false)...)
		}
	}

	// Deduplicate: view lists include $ref-expanded globals, so the same
	// component can appear multiple times. Keep first occurrence of each
	// (rule, component, selector) triplet.
	type warnKey struct{ rule, comp, sel string }
	seen := make(map[warnKey]bool, len(warnings))
	deduped := warnings[:0]
	for _, w := range warnings {
		k := warnKey{w.Rule, w.Component, w.Selector}
		if !seen[k] {
			seen[k] = true
			deduped = append(deduped, w)
		}
	}
	return deduped
}

// hasTopLevelComma reports whether s contains a comma at parenthesis depth 0.
// Commas inside functional pseudo-classes like :is(a, b) are not top-level.
func hasTopLevelComma(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// isSingletonName reports whether a component name suggests a singleton scope.
// Singletons like SiteHeader, NavFooter, etc. are expected to have 1 instance.
func isSingletonName(name string) bool {
	// Common singleton patterns
	singletonKeywords := []string{
		"Header", "Footer", "Nav", "Sidebar", "Main",
		"Site", "Page", "Layout", "Modal", "Dialog",
	}
	for _, kw := range singletonKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// hasUniquenessAnchor reports whether a selector is anchored to a unique element.
// Selectors with #id, [data-testid="exact"], [aria-labelledby], or a
// colon-anchored data-component substring are considered unique.
func hasUniquenessAnchor(selStr string) bool {
	// Quick checks for common uniqueness patterns
	if strings.Contains(selStr, "#") {
		return true // ID selector
	}
	if strings.Contains(selStr, "[data-testid=") {
		return true // data-testid with exact match
	}
	if strings.Contains(selStr, "[aria-labelledby") {
		return true // aria-labelledby (references unique ID)
	}
	if strings.Contains(selStr, "[id=") {
		return true // attribute selector on id
	}
	// [data-component*=":Name:"] — colon-anchored substring in a versioned component
	// registry attribute (format: module:ComponentName:version). The leading colon
	// ensures the match is not a prefix of a longer name (e.g. ":ShareBelt:" will not
	// match ":ShareBeltFavorite:"). Treat as specific.
	if strings.Contains(selStr, `[data-component*=":`) || strings.Contains(selStr, `[data-component*=':`) {
		return true
	}
	return false
}

// isBroadSelector reports whether a selector is likely to match multiple instances.
// We use conservative heuristics to avoid false positives:
// - Bare element tags (button, input, a)
// - Common utility classes that appear multiple times (btn, card, link)
// - Generic data-component attributes without instance-specific values
func isBroadSelector(selStr string) bool {
	// Parse to check if it's a bare tag or generic pattern
	if ps, err := sel.ParseSightmapSelector(selStr); err == nil && len(ps.Parts) > 0 {
		// Check the last part (most specific)
		lastPart := ps.Parts[len(ps.Parts)-1]

		// Bare tag with generic utility class patterns
		if lastPart.Tag != "" {
			genericTags := []string{"button", "input", "a", "div", "span"}
			for _, tag := range genericTags {
				if lastPart.Tag == tag {
					// If it's a generic tag with utility classes, it's broad
					if len(lastPart.Classes) > 0 {
						for _, cls := range lastPart.Classes {
							// Common utility class patterns
							if strings.HasPrefix(cls, "btn") ||
								strings.Contains(cls, "button") ||
								strings.Contains(cls, "card") ||
								strings.Contains(cls, "link") ||
								strings.Contains(cls, "item") {
								return true
							}
						}
					}
					// Bare generic tag (e.g., just "button")
					if len(lastPart.Classes) == 0 && lastPart.Id == "" && len(lastPart.Attrs) == 0 {
						return true
					}
				}
			}
		}

		// Generic data-component patterns
		for k := range lastPart.Attrs {
			if k == "data-component" {
				return true // data-component typically matches multiple instances
			}
		}
	}
	return false
}

// LintWithCounts is like Lint but augments the multi-instance-no-property
// warning with actual match counts drawn from snapshot data.
// counts maps component name → max match count across all provided snapshots.
// Behaviour by count value:
//   - count == 1: warning suppressed (static heuristic was a false positive)
//   - count == 0: warning kept, message appended with "(0 matches in snapshot — selector may be broken)"
//   - count > 1:  warning kept, message appended with "(matched N times in snapshot)"
//
// When counts is nil or a component name is absent from the map, the
// behaviour is identical to Lint.
func LintWithCounts(c *Corpus, counts map[string]int) []LintWarning {
	var warnings []LintWarning

	for _, ve := range Validate(c) {
		warnings = append(warnings, LintWarning{
			File:      ve.File,
			Component: ve.Component,
			Selector:  ve.Selector,
			Rule:      "validation",
			Message:   ve.Error(),
		})
	}

	for _, comp := range c.GlobalComponents {
		warnings = append(warnings, lintComponentWithCounts(comp, true, counts)...)
	}

	for _, view := range c.Views {
		for _, comp := range view.Components {
			warnings = append(warnings, lintComponentWithCounts(comp, false, counts)...)
		}
	}

	type warnKey struct{ rule, comp, sel string }
	seen := make(map[warnKey]bool, len(warnings))
	deduped := warnings[:0]
	for _, w := range warnings {
		k := warnKey{w.Rule, w.Component, w.Selector}
		if !seen[k] {
			seen[k] = true
			deduped = append(deduped, w)
		}
	}
	return deduped
}

// lintComponent runs per-component lint rules.
// global controls whether rules restricted to global scope are applied.
func lintComponent(comp match.ComponentDef, global bool) []LintWarning {
	return lintComponentWithCounts(comp, global, nil)
}

// lintComponentWithCounts is the full implementation of per-component lint
// rules. counts (component name → match count) optionally augments the
// multi-instance-no-property warning; pass nil to use static heuristics only.
func lintComponentWithCounts(comp match.ComponentDef, global bool, counts map[string]int) []LintWarning {
	var warnings []LintWarning

	name := comp.Name
	if name == "" {
		return warnings // validation catches empty names; no named rules apply
	}

	// name-case: component name should start with an uppercase letter.
	if !unicode.IsUpper(rune(name[0])) {
		warnings = append(warnings, LintWarning{
			Component: name,
			Rule:      "name-case",
			Message:   "component name should be PascalCase (starts with uppercase)",
		})
	}

	// multi-instance-no-property: warn if component likely matches multiple instances
	// but defines no properties to differentiate them.
	// Suppress for:
	// - components with properties
	// - singletons (by name pattern)
	// - child components (scoped to parent)
	// - components marked as stability: unstable (accepted T2 residual)
	if len(comp.Properties) == 0 && !isSingletonName(name) && len(comp.ParentChain) == 0 && comp.Stability != "unstable" {
		// Check if ANY selector is both broad AND not uniqueness-anchored
		hasBroadSelector := false
		for _, selStr := range comp.Selectors {
			if isBroadSelector(selStr) && !hasUniquenessAnchor(selStr) {
				hasBroadSelector = true
				break
			}
		}
		if hasBroadSelector {
			const baseMsg = "selector likely matches multiple instances but defines no properties to differentiate them. " +
				"Consider adding a property (e.g., label, id, or parent context), narrowing the selector, " +
				"marking as a child component scoped to a parent, or marking stability: unstable if this is an accepted T2 residual."

			count, hasCount := counts[name] // ok=false when counts is nil
			if hasCount && count == 1 {
				// Exactly one match in the snapshot: static heuristic was a false
				// positive. Suppress the warning.
			} else {
				msg := baseMsg
				if hasCount {
					if count == 0 {
						msg += " (0 matches in snapshot — selector may be broken)"
					} else {
						msg += fmt.Sprintf(" (matched %d times in snapshot)", count)
					}
				}
				warnings = append(warnings, LintWarning{
					Component: name,
					Rule:      "multi-instance-no-property",
					Message:   msg,
				})
			}
		}
	}

	for _, selStr := range comp.Selectors {
		// broad-tag-selector: bare tag name at global scope (e.g. "div", "button").
		if global {
			if ps, err := sel.ParseSightmapSelector(selStr); err == nil && len(ps.Parts) == 1 {
				p := ps.Parts[0]
				if p.Tag != "" && p.Id == "" && len(p.Classes) == 0 && len(p.Attrs) == 0 && p.Not == nil {
					warnings = append(warnings, LintWarning{
						Component: name,
						Selector:  selStr,
						Rule:      "broad-tag-selector",
						Message:   "bare tag selector at global scope matches too broadly",
					})
				}
			}
		}

		// id-hash-selector: #id that looks auto-generated.
		if autoIdRe.MatchString(selStr) {
			warnings = append(warnings, LintWarning{
				Component: name,
				Selector:  selStr,
				Rule:      "id-hash-selector",
				Message:   "selector uses potentially auto-generated #id (brittle)",
			})
		}

		// deep-nesting: ≥4 parsed selector parts (= ≥3 combinators between them).
		// Use the parsed part count, NOT raw space counting, to avoid false
		// positives from spaces inside quoted attribute values like
		// [aria-label="Link to homepage"].
		if ps, parseErr := sel.ParseSightmapSelector(selStr); parseErr == nil && len(ps.Parts) >= 4 {
			warnings = append(warnings, LintWarning{
				Component: name,
				Selector:  selStr,
				Rule:      "deep-nesting",
				Message:   "selector is deeply nested (≥4 levels); consider flattening or promoting ancestor",
			})
		}

		// comma-in-selector: a comma at depth 0 survived splitting.
		// Use paren-depth-aware check to ignore commas inside :is()/:where().
		if hasTopLevelComma(selStr) {
			warnings = append(warnings, LintWarning{
				Component: name,
				Selector:  selStr,
				Rule:      "comma-in-selector",
				Message:   "selector contains a comma — should be split into separate selectors",
			})
		}

		// child-repeats-parent: a class in an ancestor part appears again in the
		// next descendant part. This is the signature of a doubled selector produced
		// by flattenOne (loader.go) when the raw child selector was authored with the
		// parent class already included (e.g., ".parent .leaf" flattened to
		// ".parent .parent .leaf"), which matches zero nodes. Only relevant for
		// child components (non-empty ParentChain).
		if len(comp.ParentChain) > 0 {
			if ps, parseErr := sel.ParseSightmapSelector(selStr); parseErr == nil {
				if cls := firstRepeatedDescendantClass(ps); cls != "" {
					warnings = append(warnings, LintWarning{
						Component: name,
						Selector:  selStr,
						Rule:      "child-repeats-parent",
						Message: fmt.Sprintf(
							"child selector repeats ancestor class %q — child selectors are "+
								"auto-scoped under the parent's DOM subtree; drop the parent prefix",
							"."+cls,
						),
					})
				}
			}
		}
	}

	return warnings
}

// firstRepeatedDescendantClass returns the first class that appears in both an
// ancestor part and the immediately following descendant part of ps. Returns ""
// if no such repetition exists. This is the exact pattern produced by flattenOne
// when a child component's raw selector already includes the parent class.
func firstRepeatedDescendantClass(ps sel.ParsedSelector) string {
	for i := 0; i+1 < len(ps.Parts); i++ {
		if ps.Combinators[i+1] != " " {
			continue // only flag descendant combinators, not direct-child (>)
		}
		for _, ancestorCls := range ps.Parts[i].Classes {
			for _, childCls := range ps.Parts[i+1].Classes {
				if ancestorCls == childCls {
					return ancestorCls
				}
			}
		}
	}
	return ""
}
