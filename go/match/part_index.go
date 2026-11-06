package match

import (
	"slices"
	"strings"
	"sync"

	"github.com/sightmap/sightmap/go/sightmap"
)

// A part key is one requirement of a selector part that an element can be
// tested for cheaply: an id, an exact attribute value, a class, an attribute's
// presence, or an ASCII tag. Each part gets the most selective key it has, and
// the key is a necessary condition for the part to match: an element that
// lacks it cannot satisfy the part. Parts with no such requirement (*, a bare
// :is()/:not()/:has(), a non-ASCII tag) have no key and are always tested.
//
// Every part of every query is keyed, not just the first, because both halves
// of the walk need it. Fresh starts are looked up by key (queryIndex.fresh);
// without that every query is tried at every node, O(nodes × queries). And an
// in-flight state, inherited from an ancestor, is skipped at a node that lacks
// its pending part's key; on deep corpora hundreds of such states stay alive
// through a subtree and almost none of them match any given node.
type partKeys struct {
	n          int32
	byID       map[string]int32
	attrValues []attrValueKeys
	attrNames  []attrNameKey
	byClass    map[string]int32
	byTag      map[string]int32 // ASCII-lowercased tag

	// attrPrefixes keys [name^="prefix"]; these keys are only required of
	// in-flight states (partNeeds), never used to bucket fresh starts.
	attrPrefixes []attrPrefixKeys

	// Build-time lookups, so each distinct key gets one id.
	attrValueIx  map[string]int // name -> index into attrValues
	attrNameIx   map[string]int // name -> index into attrNames
	attrPrefixIx map[string]int // name -> index into attrPrefixes
}

// attrPrefixKeys holds the keys of one attribute name's required prefixes,
// and their distinct lengths, ascending, so a node's value is probed once per
// length.
type attrPrefixKeys struct {
	name     string
	byPrefix map[string]int32
	lens     []int
}

// attrValueKeys holds the keys of one attribute name's required exact values.
type attrValueKeys struct {
	name    string
	byValue map[string]int32
}

// attrNameKey is the key for an attribute name's presence (every attribute
// operator requires presence).
type attrNameKey struct {
	name string
	key  int32
}

// noKey marks a part with no indexable requirement.
const noKey = -1

func newPartKeys() *partKeys {
	return &partKeys{
		byID:         map[string]int32{},
		byClass:      map[string]int32{},
		byTag:        map[string]int32{},
		attrValueIx:  map[string]int{},
		attrNameIx:   map[string]int{},
		attrPrefixIx: map[string]int{},
	}
}

func (k *partKeys) intern(m map[string]int32, s string) int32 {
	if id, ok := m[s]; ok {
		return id
	}
	m[s] = k.n
	k.n++
	return m[s]
}

// keyOf returns p's key, assigning a new one the first time a requirement is
// seen, or noKey. The priority order (id, exact attribute, class, attribute
// presence, tag) prefers the requirements fewest elements meet.
func (k *partKeys) keyOf(p *sightmap.SelectorPart) int32 {
	if p == nil {
		return noKey
	}
	if p.Id != "" {
		return k.intern(k.byID, p.Id)
	}
	if name, ok := firstAttr(p, true); ok {
		return k.valueKey(name, p.Attrs[name])
	}
	if len(p.Classes) > 0 {
		return k.intern(k.byClass, p.Classes[0])
	}
	if name, ok := firstAttr(p, false); ok {
		if i, ok := k.attrNameIx[name]; ok {
			return k.attrNames[i].key
		}
		k.attrNameIx[name] = len(k.attrNames)
		k.attrNames = append(k.attrNames, attrNameKey{name: name, key: k.n})
		k.n++
		return k.n - 1
	}
	// Tags match ASCII case-insensitively, so keying by ASCII lowercase is
	// exact for ASCII tags; anything else has no key.
	if p.Tag != "" && isASCII(p.Tag) {
		return k.intern(k.byTag, strings.ToLower(p.Tag))
	}
	return noKey
}

// allKeysOf returns the keys of every id, exact-attribute, class and ASCII tag
// requirement p has, each a necessary condition, or nil when it has none. An
// in-flight state is skipped at a node missing any one of them.
func (k *partKeys) allKeysOf(p *sightmap.SelectorPart) []int32 {
	if p == nil {
		return nil
	}
	var out []int32
	if p.Id != "" {
		out = append(out, k.intern(k.byID, p.Id))
	}
	// Every test must hold, so the order keys are listed in does not matter.
	attr := func(name, op, v string) {
		switch {
		case op == "=":
			out = append(out, k.valueKey(name, v))
		case op == "^=" && v != "" && !sightmap.AttrValueCaseInsensitive(name):
			out = append(out, k.prefixKey(name, v))
		}
	}
	for name, v := range p.Attrs {
		op, ok := p.AttrOps[name]
		if !ok {
			op = "="
		}
		attr(name, op, v)
	}
	for _, t := range p.RepeatAttrs {
		attr(t.Key, t.Op, t.Value)
	}
	for _, c := range p.Classes {
		out = append(out, k.intern(k.byClass, c))
	}
	if p.Tag != "" && isASCII(p.Tag) {
		out = append(out, k.intern(k.byTag, strings.ToLower(p.Tag)))
	}
	return out
}

// valueKey returns the key for an [name="value"] requirement.
func (k *partKeys) valueKey(name, value string) int32 {
	i, ok := k.attrValueIx[name]
	if !ok {
		i = len(k.attrValues)
		k.attrValueIx[name] = i
		k.attrValues = append(k.attrValues, attrValueKeys{name: name, byValue: map[string]int32{}})
	}
	if sightmap.AttrValueCaseInsensitive(name) {
		// HTML compares this value ASCII case-insensitively; key the folded
		// value and let matching decide (it is exact for SVG).
		value = asciiLower(value)
	}
	return k.intern(k.attrValues[i].byValue, value)
}

// prefixKey returns the key for an [name^="prefix"] requirement. Only
// case-sensitive attributes get one: the element value the matcher reads
// (Element.Attr) then begins with prefix exactly when the test holds.
func (k *partKeys) prefixKey(name, prefix string) int32 {
	i, ok := k.attrPrefixIx[name]
	if !ok {
		i = len(k.attrPrefixes)
		k.attrPrefixIx[name] = i
		k.attrPrefixes = append(k.attrPrefixes, attrPrefixKeys{name: name, byPrefix: map[string]int32{}})
	}
	b := &k.attrPrefixes[i]
	if !slices.Contains(b.lens, len(prefix)) {
		b.lens = append(b.lens, len(prefix))
	}
	return k.intern(b.byPrefix, prefix)
}

// finish orders the attribute lookups by name, so a node's keys come out in a
// deterministic order.
func (k *partKeys) finish() {
	slices.SortFunc(k.attrValues, func(a, b attrValueKeys) int { return strings.Compare(a.name, b.name) })
	slices.SortFunc(k.attrNames, func(a, b attrNameKey) int { return strings.Compare(a.name, b.name) })
	slices.SortFunc(k.attrPrefixes, func(a, b attrPrefixKeys) int { return strings.Compare(a.name, b.name) })
	for i := range k.attrPrefixes {
		slices.Sort(k.attrPrefixes[i].lens)
	}
	k.attrValueIx, k.attrNameIx, k.attrPrefixIx = nil, nil, nil
}

// nodeKeys appends to out every key el meets. A class repeated on the element
// yields its key twice.
func (k *partKeys) nodeKeys(el *sightmap.Element, out []int32) []int32 {
	if el == nil {
		return out
	}
	if el.Id != "" {
		if id, ok := k.byID[el.Id]; ok {
			out = append(out, id)
		}
	}
	for i := range k.attrValues {
		b := &k.attrValues[i]
		if v, ok := el.Attr(b.name); ok {
			if sightmap.AttrValueCaseInsensitive(b.name) {
				v = asciiLower(v)
			}
			if id, ok := b.byValue[v]; ok {
				out = append(out, id)
			}
		}
	}
	for i := range k.attrNames {
		if el.HasAttr(k.attrNames[i].name) {
			out = append(out, k.attrNames[i].key)
		}
	}
	for i := range k.attrPrefixes {
		b := &k.attrPrefixes[i]
		v, ok := el.Attr(b.name)
		if !ok {
			continue
		}
		for _, n := range b.lens {
			if n > len(v) {
				break
			}
			if id, ok := b.byPrefix[v[:n]]; ok {
				out = append(out, id)
			}
		}
	}
	for _, c := range el.Classes {
		if id, ok := k.byClass[c]; ok {
			out = append(out, id)
		}
	}
	// A non-ASCII tag never ASCII-folds to a keyed (ASCII) one.
	if el.Tag != "" && isASCII(el.Tag) {
		if id, ok := k.byTag[asciiLower(el.Tag)]; ok {
			out = append(out, id)
		}
	}
	return out
}

// queryIndex is a compiled query set's keys, by dense part id
// (partBase[qi]+idx): each part's bucket key and every key it needs, and the
// queries whose first part has each bucket key.
type queryIndex struct {
	keys      *partKeys
	partBase  []int32                 // query index -> id of its Parts[0]
	partKey   []int32                 // part id -> bucket key, or noKey
	partNeeds [][]int32               // part id -> every key it requires
	parts     []*sightmap.PartMatcher // part id -> its prepared matcher
	byKey     [][]int                 // key -> queries whose first part has it, ascending
	universal []int                   // queries whose first part has no key, ascending
	// walkers pools walk scratch sized to this index (see walkMatches).
	walkers sync.Pool
}

func newQueryIndex(queries []MatchQuery) *queryIndex {
	ix := &queryIndex{keys: newPartKeys(), partBase: make([]int32, len(queries))}
	var first []int32
	for qi := range queries {
		ix.partBase[qi] = int32(len(ix.partKey))
		for _, p := range queries[qi].Parts {
			ix.partKey = append(ix.partKey, ix.keys.keyOf(p))
			ix.partNeeds = append(ix.partNeeds, ix.keys.allKeysOf(p))
			var pm *sightmap.PartMatcher
			if p != nil {
				pm = sightmap.NewPartMatcher(p)
			}
			ix.parts = append(ix.parts, pm)
		}
		fk := int32(noKey)
		if len(queries[qi].Parts) > 0 {
			fk = ix.partKey[ix.partBase[qi]]
		}
		first = append(first, fk)
	}
	ix.keys.finish()
	ix.byKey = make([][]int, ix.keys.n)
	for qi, k := range first {
		if len(queries[qi].Parts) == 0 {
			continue // nothing to match (ParseQueries never builds one)
		}
		if k == noKey {
			ix.universal = append(ix.universal, qi)
		} else {
			ix.byKey[k] = append(ix.byKey[k], qi)
		}
	}
	return ix
}

// fresh appends to out, in ascending query order, every query whose first
// part could match an element with keys nodeKeys.
func (ix *queryIndex) fresh(nodeKeys []int32, out []int) []int {
	out = append(out, ix.universal...)
	for _, k := range nodeKeys {
		out = append(out, ix.byKey[k]...)
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
