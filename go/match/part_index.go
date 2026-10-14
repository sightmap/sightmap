package match

import (
	"slices"
	"sort"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// partIndex buckets queries by one requirement of one of their selector
// parts, so a matcher only tries a query at an element that could satisfy that
// part. Without it every query is tried at every node, which is O(nodes ×
// queries) and dominated matching against real corpora, where most selectors
// are [data-component="X"] and any given node carries at most one such value.
//
// The tree walk keys each query on its first part, where a fresh match starts
// (newFirstPartIndex); chain resolution keys it on its subject, the last part,
// which the element under test must match itself (newSubjectIndex).
//
// Each query lands in exactly one bucket, keyed by the most selective
// requirement the keyed part has. A bucket key is a necessary condition for the
// part to match, so skipping a query whose bucket the node misses never drops a
// match. Queries with no indexable requirement (*, a bare :is()/:not()/:has(),
// a non-ASCII tag) are always tried.
type partIndex struct {
	byID       map[string][]int
	attrValues []attrValueBucket
	attrNames  []attrNameBucket
	byClass    map[string][]int
	byTag      map[string][]int // ASCII-lowercased tag
	universal  []int
}

// attrValueBucket indexes queries whose keyed part requires name=value.
type attrValueBucket struct {
	name    string
	byValue map[string][]int
}

// attrNameBucket indexes queries whose keyed part requires the attribute name
// to be present (every attribute operator requires presence).
type attrNameBucket struct {
	name    string
	queries []int
}

// newFirstPartIndex keys each query on its first part.
func newFirstPartIndex(queries []MatchQuery) *partIndex {
	return newPartIndex(queries, func(q *MatchQuery) *sightmap.SelectorPart { return q.Parts[0] })
}

// newSubjectIndex keys each query on its subject, its last part.
func newSubjectIndex(queries []MatchQuery) *partIndex {
	return newPartIndex(queries, func(q *MatchQuery) *sightmap.SelectorPart { return q.Parts[len(q.Parts)-1] })
}

// newPartIndex keys each query on the part keyed returns for it.
func newPartIndex(queries []MatchQuery, keyed func(*MatchQuery) *sightmap.SelectorPart) *partIndex {
	ix := &partIndex{
		byID:    map[string][]int{},
		byClass: map[string][]int{},
		byTag:   map[string][]int{},
	}
	attrValues := map[string]map[string][]int{}
	attrNames := map[string][]int{}

	for qi := range queries {
		var p *sightmap.SelectorPart
		if len(queries[qi].Parts) > 0 {
			p = keyed(&queries[qi])
		}
		switch k := keyOf(p); k.kind {
		case keyID:
			ix.byID[k.name] = append(ix.byID[k.name], qi)
		case keyAttrValue:
			if attrValues[k.name] == nil {
				attrValues[k.name] = map[string][]int{}
			}
			attrValues[k.name][k.value] = append(attrValues[k.name][k.value], qi)
		case keyClass:
			ix.byClass[k.name] = append(ix.byClass[k.name], qi)
		case keyAttrName:
			attrNames[k.name] = append(attrNames[k.name], qi)
		case keyTag:
			ix.byTag[k.name] = append(ix.byTag[k.name], qi)
		default:
			ix.universal = append(ix.universal, qi)
		}
	}

	for _, name := range sortedKeys(attrValues) {
		ix.attrValues = append(ix.attrValues, attrValueBucket{name: name, byValue: attrValues[name]})
	}
	for _, name := range sortedKeys(attrNames) {
		ix.attrNames = append(ix.attrNames, attrNameBucket{name: name, queries: attrNames[name]})
	}
	return ix
}

// candidates appends to out, in ascending query order, every query whose keyed
// part el could satisfy.
func (ix *partIndex) candidates(el *sightmap.Element, out []int) []int {
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

// partKey is the one requirement of a selector part that indexes it: the most
// selective the part has. It is a necessary condition for the part to match.
type partKey struct {
	kind  keyKind
	name  string // the id, attribute name, class, or ASCII-lowercased tag
	value string // the attribute value, ASCII-lowercased where HTML folds it
}

type keyKind uint8

const (
	keyUniversal keyKind = iota // no indexable requirement: always a candidate
	keyID
	keyAttrValue
	keyClass
	keyAttrName
	keyTag
)

// keyOf picks p's key: its id, else an exact attribute value, else a class,
// else a required attribute, else an ASCII tag.
func keyOf(p *sightmap.SelectorPart) partKey {
	if p == nil {
		return partKey{}
	}
	if p.Id != "" {
		return partKey{kind: keyID, name: p.Id}
	}
	if name, ok := firstAttr(p, true); ok {
		v := p.Attrs[name]
		if sightmap.AttrValueCaseInsensitive(name) {
			// HTML compares this value ASCII case-insensitively; key on the
			// folded value and let matching decide (it is exact for SVG).
			v = asciiLower(v)
		}
		return partKey{kind: keyAttrValue, name: name, value: v}
	}
	if len(p.Classes) > 0 {
		return partKey{kind: keyClass, name: p.Classes[0]}
	}
	if name, ok := firstAttr(p, false); ok {
		return partKey{kind: keyAttrName, name: name}
	}
	// Tags match ASCII case-insensitively, so keying by ASCII lowercase is exact
	// for ASCII tags; anything else stays universal.
	if p.Tag != "" && isASCII(p.Tag) {
		return partKey{kind: keyTag, name: strings.ToLower(p.Tag)}
	}
	return partKey{}
}

// admits reports whether el meets k, exactly as candidates would select a
// query keyed on k for el. It is a cheap check before full part matching.
func (k *partKey) admits(el *sightmap.Element) bool {
	switch k.kind {
	case keyID:
		return el.Id == k.name
	case keyAttrValue:
		v, ok := el.Attr(k.name)
		if ok && sightmap.AttrValueCaseInsensitive(k.name) {
			v = asciiLower(v)
		}
		return ok && v == k.value
	case keyClass:
		return slices.Contains(el.Classes, k.name)
	case keyAttrName:
		return el.HasAttr(k.name)
	case keyTag:
		return isASCII(el.Tag) && asciiLower(el.Tag) == k.name
	}
	return true
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
