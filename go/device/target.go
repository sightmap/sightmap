package device

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

var (
	snapshotID   = regexp.MustCompile(`^\d+$`)
	bareWord     = regexp.MustCompile(`^[^\s\[\]"'#]+$`)
	trailingNth  = regexp.MustCompile(`^(.*\S)#(\d+)$`)
	selectorHint = "a selector over the normalized tree, e.g. `button[identifier=\"login-submit\"]`, `[text=\"Not Now\"]`"
)

// IsSnapshotID reports whether target names a node by its per-snapshot id.
func IsSnapshotID(target string) bool { return snapshotID.MatchString(strings.TrimSpace(target)) }

// Resolve finds the one node target names in s. A target is a snapshot id
// (`12`), an exact accessibility identifier (`login-submit`), a component query
// (`SignInButton`, `ProductRow[title^="Product 15"]`, `LoginForm SignInButton`,
// with `#N` picking an occurrence), or, for elements no component names, a
// selector over the normalized tree (`button[identifier="login-submit"]`,
// `[text="Not Now"]#0`).
func Resolve(s *Snapshot, target string) (*sightmap.ComponentNode, error) {
	text := strings.TrimSpace(target)
	if text == "" {
		return nil, errorf("bad_query", "empty target")
	}
	if snapshotID.MatchString(text) {
		if n := s.Node(text); n != nil {
			return n, nil
		}
		return nil, errorf("not_found", "no element with id %s in this snapshot; take a new snapshot", text)
	}
	if bareWord.MatchString(text) {
		var byID []*sightmap.ComponentNode
		for _, n := range s.Nodes {
			if Identifier(n) == text {
				byID = append(byID, n)
			}
		}
		switch {
		case len(byID) == 1:
			return byID[0], nil
		case len(byID) > 1:
			return nil, s.ambiguous(fmt.Sprintf("identifier %s is on %d elements; use a snapshot id or `%s[identifier=%s]#N`", quote(text), len(byID), byID[0].Role, quote(text)), byID)
		}
	}
	if unicode.IsUpper(rune(text[0])) {
		return s.resolveComponent(text)
	}
	return s.resolveSelector(text)
}

func (s *Snapshot) resolveComponent(text string) (*sightmap.ComponentNode, error) {
	q, err := compquery.ParseQuery(text)
	if err != nil {
		return nil, errorf("bad_query", "cannot parse component query %s: %v", quote(text), err)
	}
	found := compquery.FindCandidates(s.Root, s.Matches, s.Props, q)
	return s.pick(text, found, q.Index, func() string {
		name := q.Parts[len(q.Parts)-1].Name
		for _, cm := range s.Matches {
			if cm.Name == name {
				return ""
			}
		}
		return fmt.Sprintf(" (no %s component in this snapshot)", name)
	})
}

func (s *Snapshot) resolveSelector(text string) (*sightmap.ComponentNode, error) {
	selector, nth := text, -1
	if m := trailingNth.FindStringSubmatch(text); m != nil {
		selector = m[1]
		nth, _ = strconv.Atoi(m[2])
	}
	queries, errs := match.ParseQueries([]sightmap.ComponentDef{{Name: "target", Selectors: []string{selector}}})
	if len(errs) > 0 || len(queries) == 0 {
		return nil, withHint("bad_query", "targets are a snapshot id, an identifier, a component query (`SignInButton`, `ProductRow[title^=\"Product 1\"]`), or "+selectorHint,
			"cannot parse %s as a selector", quote(text))
	}
	var found []*sightmap.ComponentNode
	seen := map[*sightmap.ComponentNode]bool{}
	match.FindAllMatches(s.Root, queries, func(n *sightmap.ComponentNode, _ *match.MatchQuery) {
		if !seen[n] {
			seen[n] = true
			found = append(found, n)
		}
	})
	return s.pick(text, found, nth, func() string { return "" })
}

// pick returns the single node in found, or occurrence nth (0-based) when it is
// set; why explains an empty result.
func (s *Snapshot) pick(text string, found []*sightmap.ComponentNode, nth int, why func() string) (*sightmap.ComponentNode, error) {
	if nth >= 0 {
		if nth < len(found) {
			return found[nth], nil
		}
		return nil, errorf("not_found", "%s has %d match(es); occurrence #%d does not exist", quote(text), len(found), nth)
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, errorf("not_found", "no element matches %s%s", quote(text), why())
	}
	return nil, s.ambiguous(fmt.Sprintf("%s matches %d elements; add a predicate or append #N", quote(text), len(found)), found)
}

// ambiguousError is an "ambiguous" Error that keeps the elements matched.
type ambiguousError struct {
	err   *Error
	nodes []*sightmap.ComponentNode
}

func (e *ambiguousError) Error() string { return e.err.Error() }
func (e *ambiguousError) Unwrap() error { return e.err }

func (s *Snapshot) ambiguous(msg string, nodes []*sightmap.ComponentNode) error {
	lines := make([]string, len(nodes))
	for i, n := range nodes {
		lines[i] = s.line(n)
	}
	return &ambiguousError{withHint("ambiguous", "pick one by snapshot id or #N", "%s:\n  %s", msg, strings.Join(lines, "\n  ")), nodes}
}
