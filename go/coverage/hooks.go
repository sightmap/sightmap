package coverage

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// Candidate generation for orphaned nodes. Where the anchor helpers (anchor.go)
// recognize ONLY data-testid/data-component, these surface the full range of
// stable hooks a framework-generated DOM actually offers — custom-element tags,
// design-system classes, ids, name/href/aria attrs — so `gap` stops dead-ending
// with "(no stable ancestor) → (no selector hint)" on hook-poor apps (Salesforce
// Lightning, Angular, etc.). data-* hooks are still ranked highest; they are one
// input to candidate generation, not an override that suppresses everything else.
//
// The philosophy is surface-and-rank, not auto-select: emit the plausible stable
// candidates, ranked, and let the author verify with sel-probe (which the
// authoring skill already mandates before any selector reaches YAML). Only
// clearly machine-generated tokens are dropped.

var (
	// A run of 4+ digits: counters, timestamps, epoch-ish ids.
	reDigitRun = regexp.MustCompile(`[0-9]{4,}`)
	// A long hex run: hashes, uuids, content-addressed names.
	reHexRun = regexp.MustCompile(`[0-9a-fA-F]{8,}`)
	// data-component version suffix, e.g. ":v1.2.3" or ":v1.2.3-abc".
	reComponentVersion = regexp.MustCompile(`:v\d+\.\d+\.\d+.*$`)
	// A trailing numeric instance counter, e.g. "combobox-button-15", "tab_3".
	reTrailingNum = regexp.MustCompile(`^(.+?)[-_]?\d+$`)
	// Leading authored prefix before any digit — for [id^="prefix"] forms.
	reLeadingPrefix = regexp.MustCompile(`^([A-Za-z][A-Za-z_-]{2,}?)[-_]?\d`)
)

// utilityClasses are presentational/layout tokens that make poor SOLE selectors.
// They are ranked lower, never dropped — sometimes they are the only hook.
var utilityClasses = map[string]bool{
	"active": true, "open": true, "closed": true, "hidden": true, "show": true,
	"hide": true, "selected": true, "disabled": true, "visible": true,
	"container": true, "row": true, "col": true, "wrapper": true, "content": true,
	"clearfix": true, "sr-only": true,
}

// looksHashed reports whether a token is likely machine-generated / per-instance
// (Aura render ids like "1:1;a", hashed classnames, uuids, emotion/styled-
// components suffixes) rather than a stable authored hook worth suggesting.
func looksHashed(tok string) bool {
	if tok == "" {
		return true
	}
	if strings.ContainsAny(tok, ":;$") { // aura ids "1:1;a", scoped "$x"
		return true
	}
	if reDigitRun.MatchString(tok) {
		return true
	}
	if reHexRun.MatchString(tok) {
		return true
	}
	// A hash-like segment (word-3k9f2h1): >=6 chars mixing letters with several
	// digits. Catches emotion/styled/LWC-scoped suffixes that dodge the run
	// checks, while sparing short utilities (col12, step2) and pure words.
	for _, seg := range strings.FieldsFunc(tok, func(r rune) bool { return r == '-' || r == '_' }) {
		if len(seg) >= 6 && hasLetter(seg) && countDigits(seg) >= 2 {
			return true
		}
	}
	return false
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

func stripComponentVersion(v string) string {
	return reComponentVersion.ReplaceAllString(v, "")
}

// idPrefix returns the stable leading prefix of an id that carries a trailing
// counter/hash, for use in an [id^="prefix"] selector. "" if none usable.
func idPrefix(id string) string {
	if m := reLeadingPrefix.FindStringSubmatch(id); m != nil {
		return m[1]
	}
	return ""
}

// hrefSuffix extracts a portable, stable trailing path segment from an href, for
// an a[href$="…"] selector. Returns "" for hrefs whose tail is dynamic (numeric
// or hashed) or that carry no usable path. Prefers the suffix form because a
// full href embeds site/workspace-specific prefixes (see dc26).
func hrefSuffix(href string) string {
	h := href
	if i := strings.IndexAny(h, "?#"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimRight(h, "/")
	slash := strings.LastIndex(h, "/")
	if slash < 0 {
		return ""
	}
	seg := h[slash+1:]
	if seg == "" || looksHashed(seg) {
		return ""
	}
	return "/" + seg
}

// SelectorCandidates returns ranked stable selector candidates identifying el,
// best first (up to 4). data-* hooks rank highest but never suppress the rest.
// Clearly machine-generated tokens are dropped; callers verify with sel-probe.
func SelectorCandidates(el *sightmap.Element) []string {
	if el == nil {
		return nil
	}
	type cand struct {
		sel   string
		score int
	}
	var cands []cand
	seen := map[string]bool{}
	add := func(sel string, score int) {
		if sel == "" || seen[sel] {
			return
		}
		seen[sel] = true
		cands = append(cands, cand{sel, score})
	}

	tag := strings.ToLower(el.Tag)
	a := el.Attrs

	// Purpose-built test hooks (highest, but only one input).
	if v := a["data-testid"]; v != "" && !looksHashed(v) {
		add(fmt.Sprintf(`[data-testid="%s"]`, v), 100)
	}
	if v := a["data-component"]; v != "" {
		add(fmt.Sprintf(`[data-component^="%s"]`, stripComponentVersion(v)), 95)
	}
	// Custom-element tag (web components) — stable and semantic.
	if strings.Contains(tag, "-") {
		add(tag, 80)
	}
	// Stable id.
	if id := el.Id; id != "" {
		switch {
		case looksHashed(id):
			if pre := idPrefix(id); pre != "" {
				add(fmt.Sprintf(`[id^="%s"]`, pre), 45)
			}
		case reTrailingNum.MatchString(id):
			// per-instance counter: prefer the prefix form
			if pre := idPrefix(id); pre != "" {
				add(fmt.Sprintf(`[id^="%s"]`, pre), 55)
			} else {
				add("#"+id, 60)
			}
		default:
			add("#"+id, 70)
		}
	}
	// Other stable data-* attributes (e.g. data-target-selection-name).
	for k, v := range a {
		if k == "data-testid" || k == "data-component" || !strings.HasPrefix(k, "data-") {
			continue
		}
		if v == "" || looksHashed(v) {
			continue
		}
		add(fmt.Sprintf(`[%s="%s"]`, k, v), 55)
	}
	// Form-control name.
	if v := a["name"]; v != "" && !looksHashed(v) {
		add(fmt.Sprintf(`%s[name="%s"]`, tag, v), 65)
	}
	// Stable classes, combined with the tag for specificity.
	for _, cls := range el.Classes {
		if cls == "" || looksHashed(cls) {
			continue
		}
		score := 60
		if utilityClasses[cls] {
			score = 30
		}
		if strings.ContainsAny(cls, "-_") { // BEM/kebab reads authored
			score += 5
		}
		if tag != "" {
			add(tag+"."+cls, score)
		} else {
			add("."+cls, score)
		}
	}
	// Link href suffix.
	if v := a["href"]; v != "" {
		if suf := hrefSuffix(v); suf != "" {
			add(fmt.Sprintf(`a[href$="%s"]`, suf), 40)
		}
	}
	// aria-label as a last resort (usually better as a property than a selector).
	if v := a["aria-label"]; v != "" && !looksHashed(v) && len(v) <= 40 {
		add(fmt.Sprintf(`%s[aria-label="%s"]`, tag, v), 20)
	}

	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	var out []string
	for _, c := range cands {
		out = append(out, c.sel)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// NearestHookAncestor walks up to the nearest ancestor that has any stable
// selector candidate, returning it and its best candidate. The broadened
// analogue of NearestDataAttrAncestor (data-attr-only): it gives `gap` a
// container hook to scope a leaf candidate even when no data-attr ancestor
// exists. "" when nothing up the chain carries a stable hook.
func NearestHookAncestor(node *sightmap.ComponentNode, parentMap ParentMap) (*sightmap.ComponentNode, string) {
	for curr := parentMap[node]; curr != nil; curr = parentMap[curr] {
		if curr.Element == nil {
			continue
		}
		if cands := SelectorCandidates(curr.Element); len(cands) > 0 {
			return curr, cands[0]
		}
	}
	return nil, ""
}
