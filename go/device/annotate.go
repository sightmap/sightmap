package device

import (
	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Snapshot is a Tree with a corpus applied: the view it is on, the component
// each node matched, their property values and coverage.
type Snapshot struct {
	*Tree
	View *sightmap.ViewDef
	// ViewInferred is true when the view was inferred from the tree rather than
	// named by the caller.
	ViewInferred bool
	Matches      map[*sightmap.ComponentNode]*sightmap.ComponentMatch
	// Props holds each matched node's property values, keyed by node id.
	Props     map[string]map[string]string
	Conflicts []sightmap.Conflict
	Coverage  coverage.Result
	Memory    []Memory
	// CorpusApplied is false when there was no corpus to apply.
	CorpusApplied bool
}

// Memory is one memory entry that applies to the snapshot; Scope names the view
// or component it came from, empty for corpus-wide memory.
type Memory struct {
	Scope string `json:"scope,omitempty"`
	Text  string `json:"text"`
}

// Annotate applies corpus to t. A native screen has no URL to route on, so the
// view is the one named by view, or else the one whose own components match the
// most nodes (see InferView). A nil corpus yields an unannotated snapshot.
func Annotate(t *Tree, corpus *sightmap.Corpus, m *match.Matcher, view string) (*Snapshot, error) {
	s := &Snapshot{Tree: t, Props: map[string]map[string]string{}}
	if corpus == nil {
		s.Coverage = coverage.Score(t.Root, nil, coverage.Options{VisibleOnly: true})
		return s, nil
	}
	if m == nil {
		m = match.NewMatcher(corpus)
	}
	s.CorpusApplied = true
	if view != "" {
		s.View = corpus.ViewByName(view)
		if s.View == nil {
			return nil, errorf("unknown_view", "no view named %q in the corpus", view)
		}
	} else {
		s.View = InferView(t, corpus, m)
		s.ViewInferred = s.View != nil
	}
	s.Matches = m.MatchView(t.Root, s.View)
	s.Conflicts = m.ConflictsView(t.Root, s.View)
	s.Coverage = coverage.Score(t.Root, s.Matches, coverage.Options{VisibleOnly: true})
	for n, cm := range s.Matches {
		props := map[string]string{}
		for _, p := range cm.Properties {
			props[p.Name] = p.Value
		}
		s.Props[n.Id] = props
	}
	s.Memory = memoryFor(corpus, s.View, t, s.Matches)
	return s, nil
}

// InferView returns the view whose own components (not globals, nor globals it
// references) match the most nodes in t; the first declared wins a tie, and nil
// means no view's own components match anything. This stands in for route
// matching, which needs a URL a native screen does not have.
func InferView(t *Tree, corpus *sightmap.Corpus, m *match.Matcher) *sightmap.ViewDef {
	var best *sightmap.ViewDef
	bestScore := 0
	for i := range corpus.Views {
		v := &corpus.Views[i]
		own := map[string]bool{}
		for _, c := range v.Components {
			if c.Origin == "" {
				own[c.Name] = true
			}
		}
		score := 0
		for _, cm := range m.MatchView(t.Root, v) {
			if own[cm.Name] {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = v, score
		}
	}
	return best
}

func memoryFor(corpus *sightmap.Corpus, view *sightmap.ViewDef, t *Tree, matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch) []Memory {
	var out []Memory
	for _, text := range corpus.Memory {
		out = append(out, Memory{Text: text})
	}
	if view != nil {
		for _, text := range view.Memory {
			out = append(out, Memory{Scope: view.Name, Text: text})
		}
	}
	seen := map[string]bool{}
	for _, n := range t.Nodes {
		cm := matches[n]
		if cm == nil || seen[cm.Name] {
			continue
		}
		seen[cm.Name] = true
		for _, text := range cm.Memory {
			out = append(out, Memory{Scope: cm.Name, Text: text})
		}
	}
	return out
}

// Component returns the name of the component n matched, or "".
func (s *Snapshot) Component(n *sightmap.ComponentNode) string {
	if cm := s.Matches[n]; cm != nil {
		return cm.Name
	}
	return ""
}

// ViewName returns the snapshot's view name, or "".
func (s *Snapshot) ViewName() string {
	if s.View == nil {
		return ""
	}
	return s.View.Name
}
