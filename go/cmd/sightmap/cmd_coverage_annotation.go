package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/comps"
	"github.com/sightmap/sightmap/go/match"
)

// Annotation-completeness detector
// --------------------------------------------
// T1/T2/T3 coverage only scores INTERACTIVE nodes, so a NON-interactive content
// node whose accessible name carries real information — e.g. a banner `image
// "UP TO $800 OFF…"` — is structurally invisible to every coverage tier: it is
// neither a clickable orphan (T3) nor a scoped control (T2). The only way such a
// gap surfaced before was a human noticing it. This detector surfaces that class
// automatically.
//
// A node is an UNCAPTURED NAMED CONTENT NODE when ALL of:
//  1. it has a non-trivial accessible name (see isContentName),
//  2. it is not itself matched by a component (not T1), and
//  3. it has no matched ancestor at all — zero component context.
//
// Requiring zero component context (3) makes the check precise by construction:
// a named text node INSIDE a matched component (e.g. a price inside a matched
// ProductPod, possibly captured by a property) is never flagged. The empirical
// driver — the banner image with no component context — is flagged pre-fix; once
// a BannerImage component matches it the node becomes T1 (2 fails) and is no
// longer flagged, which is exactly the yak's acceptance.
//
// This is a NUDGE, not a gate: it prints an advisory section only and never
// affects coverage's exit status. Bias toward precision so the signal stays
// trustworthy.

// minContentNameLen is the shortest accessible name length (in runes, after
// trimming) we treat as "real content" rather than an icon label or short
// caption. Together with the letter requirement in isContentName it biases the
// detector toward banner titles and promo copy. Conservative by design.
const minContentNameLen = 12

// contentGap is one flagged uncaptured named content node within a single
// capture. Selector is the nearest data-attr ancestor's selector (the same
// anchor used by the coverage trace / orphanSlotKey), or "" when there is none.
type contentGap struct {
	Role     string // node role, e.g. "image", "heading"
	Name     string // trimmed accessible name
	Selector string // nearest data-attr ancestor selector, or "" if none
}

// isContentName reports whether s is a non-trivial accessible name worth
// treating as real content: after trimming it must be at least minContentNameLen
// runes AND contain a letter. The length floor drops short labels; the letter
// requirement drops pure-symbol / pure-number strings. Both bias toward banner
// titles and promo copy and away from icons.
func isContentName(s string) bool {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) < minContentNameLen {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// annotationGaps returns the uncaptured named content nodes in one capture's
// tree. Interactive nodes are skipped here because they are already scored by
// the T1/T2/T3 tiers (an interactive orphan is a T3, reported elsewhere); this
// detector exists to catch the NON-interactive content that coverage cannot see.
// visibleOnly mirrors computeCoverage: hidden/ignored nodes are skipped when set.
func annotationGaps(
	root *comps.ComponentNode,
	matches map[*comps.ComponentNode]*match.SightmapMatch,
	parentMap map[*comps.ComponentNode]*comps.ComponentNode,
	visibleOnly bool,
) []contentGap {
	var gaps []contentGap
	if root == nil {
		return gaps
	}
	comps.Walk(root, func(n *comps.ComponentNode, _ int) bool {
		if n.IsInteractive {
			return true // scored by T1/T2/T3 already
		}
		if visibleOnly && (!n.IsVisible || n.IsIgnored) {
			return true
		}
		if !isContentName(n.Name) {
			return true
		}
		if matches[n] != nil {
			return true // T1: directly captured by a component
		}
		if nearestMatchedAncestor(n, parentMap, matches) != nil {
			return true // has component context → precision guard, not a gap
		}
		g := contentGap{Role: n.Role, Name: strings.TrimSpace(n.Name)}
		if anc := nearestDataAttrAncestor(n, parentMap); anc != nil {
			g.Selector = dataAttrSel(anc)
		}
		gaps = append(gaps, g)
		return true
	})
	return gaps
}

// contentGapPresence is a distinct annotation gap aggregated across a view's
// capture set. Rotating content means the same gap may appear in
// many captures, so gaps are deduped by gapKey and reported once with the count
// of captures they appeared in (mirroring the [Presence] "K of N snaps" style).
type contentGapPresence struct {
	Gap        contentGap
	AppearedIn int // captures in which this gap was flagged
	Captures   int // size of the set (N)
}

// normalizeGapName collapses whitespace and lowercases a name so trivially
// different renderings of the same banner copy dedup to one gap.
func normalizeGapName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// gapKey is the stable cross-capture identity of a content gap: role +
// normalized name + nearest-data-attr-ancestor selector. Two flagged nodes with
// the same key are the same gap seen in different captures.
func gapKey(g contentGap) string {
	return g.Role + "\x00" + normalizeGapName(g.Name) + "\x00" + g.Selector
}

// unionContentGaps aggregates per-capture content gaps across a view's set into
// distinct gaps, each with the count of captures it appeared in. capGaps holds
// one slice per capture (in set order); a gap is counted at most once per
// capture. The result is sorted by descending appearance count, then by key for
// stable output.
func unionContentGaps(capGaps [][]contentGap) []contentGapPresence {
	n := len(capGaps)
	type agg struct {
		gap   contentGap
		count int
	}
	var order []string
	byKey := make(map[string]*agg)
	for _, gaps := range capGaps {
		seen := make(map[string]bool)
		for _, g := range gaps {
			k := gapKey(g)
			if seen[k] {
				continue
			}
			seen[k] = true
			a := byKey[k]
			if a == nil {
				a = &agg{gap: g}
				byKey[k] = a
				order = append(order, k)
			}
			a.count++
		}
	}
	out := make([]contentGapPresence, 0, len(order))
	for _, k := range order {
		a := byKey[k]
		out = append(out, contentGapPresence{Gap: a.gap, AppearedIn: a.count, Captures: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AppearedIn != out[j].AppearedIn {
			return out[i].AppearedIn > out[j].AppearedIn
		}
		return gapKey(out[i].Gap) < gapKey(out[j].Gap)
	})
	return out
}

// emitContentGaps prints the [Annotation gaps] advisory section for a view: the
// distinct uncaptured named content nodes found across its capture set. It is a
// nudge — it never changes coverage's exit status. Prints nothing when there are
// no gaps.
func emitContentGaps(viewName string, pres []contentGapPresence) {
	if len(pres) == 0 {
		return
	}
	fmt.Println("[Annotation gaps] (named content with no component context)")
	for _, p := range pres {
		loc := ""
		if p.Gap.Selector != "" {
			loc = " inside " + p.Gap.Selector
		}
		name := displayName(p.Gap.Name)
		if p.Captures > 1 {
			fmt.Printf("  %s: %s %s%s \u2014 %d of %d snaps\n",
				viewName, p.Gap.Role, name, loc, p.AppearedIn, p.Captures)
		} else {
			fmt.Printf("  %s: %s %s%s\n", viewName, p.Gap.Role, name, loc)
		}
	}
	fmt.Println()
}
