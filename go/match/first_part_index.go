package match

import (
	"slices"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// firstPartIndex buckets queries by one requirement of their first selector
// part, so the matcher only tries a fresh start at a node that could satisfy
// it. Without it every query is tried at every node, which is O(nodes ×
// queries) and dominated matching against real corpora, where most selectors
// are [data-component="X"] and any given node carries at most one such value.
//
// Each query lands in exactly one bucket, keyed by the most selective
// requirement its first part has. A bucket key is a necessary condition for the
// part to match, so skipping a query whose bucket the node misses never drops a
// match. Queries with no indexable requirement (*, a bare :is()/:not()/:has(),
// a non-ASCII tag) are always tried.
type firstPartIndex struct {
	byID       map[string][]int
	attrValues []attrValueBucket
	attrNames  []attrNameBucket
	byClass    map[string][]int
	byTag      map[string][]int // ASCII-lowercased tag
	universal  []int
}

// attrValueBucket indexes queries whose first part requires name=value.
type attrValueBucket struct {
	name    string
	byValue map[string][]int
}

// attrNameBucket indexes queries whose first part requires the attribute name
// to be present (every attribute operator requires presence).
type attrNameBucket struct {
	name    string
	queries []int
}

func newFirstPartIndex(queries []MatchQuery) *firstPartIndex {
	ix := &firstPartIndex{
		byID:    map[string][]int{},
		byClass: map[string][]int{},
		byTag:   map[string][]int{},
	}
	attrValues := map[string]map[string][]int{}
	attrNames := map[string][]int{}

	for qi := range queries {
		var p *sightmap.SelectorPart
		if len(queries[qi].Parts) > 0 {
			p = queries[qi].Parts[0]
		}
		if p == nil {
			ix.universal = append(ix.universal, qi)
			continue
		}
		if p.Id != "" {
			ix.byID[p.Id] = append(ix.byID[p.Id], qi)
			continue
		}
		if name, ok := firstAttr(p, true); ok {
			if attrValues[name] == nil {
				attrValues[name] = map[string][]int{}
			}
			v := p.Attrs[name]
			if sightmap.AttrValueCaseInsensitive(name) {
				// HTML compares this value ASCII case-insensitively; bucket the
				// folded value and let matching decide (it is exact for SVG).
				v = asciiLower(v)
			}
			attrValues[name][v] = append(attrValues[name][v], qi)
			continue
		}
		if len(p.Classes) > 0 {
			ix.byClass[p.Classes[0]] = append(ix.byClass[p.Classes[0]], qi)
			continue
		}
		if name, ok := firstAttr(p, false); ok {
			attrNames[name] = append(attrNames[name], qi)
			continue
		}
		// Tags match ASCII case-insensitively, so indexing by ASCII lowercase is
		// exact for ASCII tags; anything else stays universal.
		if p.Tag != "" && isASCII(p.Tag) {
			t := strings.ToLower(p.Tag)
			ix.byTag[t] = append(ix.byTag[t], qi)
			continue
		}
		ix.universal = append(ix.universal, qi)
	}

	for _, name := range sortedKeys(attrValues) {
		ix.attrValues = append(ix.attrValues, attrValueBucket{name: name, byValue: attrValues[name]})
	}
	for _, name := range sortedKeys(attrNames) {
		ix.attrNames = append(ix.attrNames, attrNameBucket{name: name, queries: attrNames[name]})
	}
	return ix
}

// candidates appends to out, in ascending query order, every query whose first
// part el could satisfy.
func (ix *firstPartIndex) candidates(el *sightmap.Element, out []int) []int {
	out = append(out, ix.universal...)
	if el == nil {
		return out
	}
	if el.Id != "" {
		out = append(out, ix.byID[el.Id]...)
	}
	for i := range ix.attrValues {
		b := &ix.attrValues[i]
		if v, ok := el.Attr(b.name); ok {
			if sightmap.AttrValueCaseInsensitive(b.name) {
				v = asciiLower(v)
			}
			out = append(out, b.byValue[v]...)
		}
	}
	for i := range ix.attrNames {
		b := &ix.attrNames[i]
		if el.HasAttr(b.name) {
			out = append(out, b.queries...)
		}
	}
	for _, c := range el.Classes {
		out = append(out, ix.byClass[c]...)
	}
	// A non-ASCII tag never ASCII-folds to an indexed (ASCII) one.
	if el.Tag != "" && isASCII(el.Tag) {
		out = append(out, ix.byTag[asciiLower(el.Tag)]...)
	}
	// Buckets are each in query order but interleave, and a class repeated on
	// the element hits its bucket twice.
	slices.Sort(out)
	return slices.Compact(out)
}

// firstAttr returns the lexicographically first attribute name p requires,
// restricted to exact-equality attributes when exact is set. The choice only
// needs to be deterministic; any required attribute is a sound bucket key.
func firstAttr(p *sightmap.SelectorPart, exact bool) (string, bool) {
	var best string
	found := false
	for name := range p.Attrs {
		if exact {
			if op, ok := p.AttrOps[name]; ok && op != "=" {
				continue
			}
		}
		if !found || name < best {
			best, found = name, true
		}
	}
	return best, found
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiLower lowercases an ASCII string, returning s itself (no allocation)
// when it has no uppercase letters, as DOM tag names usually don't.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			return strings.ToLower(s)
		}
	}
	return s
}
