package explore

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Candidate is one actionable element offered to the picker.
type Candidate struct {
	Key     string // "n<id>"
	Node    *Node
	Desc    string
	SeenKey string // "<verb> <desc>": stable across snapshots, used for the repeat guard
}

// Meta actions offered alongside elements.
const (
	MetaBack   = "back"
	MetaScroll = "scroll"
)

var metaDesc = map[string]string{
	MetaBack:   "go back to the previous page",
	MetaScroll: "scroll down to reveal more of the page",
}

// CandidateOptions tunes candidate selection.
type CandidateOptions struct {
	Seen      map[string]int // "<url>|<seenKey>" -> times acted at that URL
	URL       string
	Avoid     []string // case-insensitive substrings; matching controls are dropped (Delete, Pay, ...)
	MaxRepeat int      // times the same action may repeat at one URL before it is hidden (default 2)
}

// Candidates lists the visible interactive nodes as candidates, dropping icons
// nested in a same-named interactive parent, avoided controls, and actions
// already repeated at this URL.
func Candidates(nodes []*Node, opts CandidateOptions) []*Candidate {
	maxRepeat := opts.MaxRepeat
	if maxRepeat <= 0 {
		maxRepeat = 2
	}
	var avoidRe *regexp.Regexp
	if len(opts.Avoid) > 0 {
		parts := make([]string, 0, len(opts.Avoid))
		for _, a := range opts.Avoid {
			if strings.TrimSpace(a) != "" {
				parts = append(parts, regexp.QuoteMeta(strings.TrimSpace(a)))
			}
		}
		if len(parts) > 0 {
			avoidRe = regexp.MustCompile("(?i)" + strings.Join(parts, "|"))
		}
	}
	var out []*Candidate
	for _, n := range nodes {
		if !n.Interactive || !n.Visible || n.Role == "image" {
			continue
		}
		if p := n.Parent; p != nil && p.Interactive && p.Name == n.Name && n.Comp == "" {
			continue
		}
		if avoidRe != nil {
			hay := n.Name + " " + n.Comp + " " + strings.Join(propValues(n), " ")
			if avoidRe.MatchString(hay) {
				continue
			}
		}
		desc := Describe(n)
		seenKey := actionVerb(n) + " " + desc
		if opts.Seen[opts.URL+"|"+seenKey] >= maxRepeat {
			continue
		}
		out = append(out, &Candidate{Key: "n" + n.ID, Node: n, Desc: desc, SeenKey: seenKey})
	}
	return out
}

func propValues(n *Node) []string {
	vals := make([]string, 0, len(n.Props))
	for _, v := range n.Props {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	return vals
}

func actionVerb(n *Node) string {
	switch {
	case IsTextInput(n):
		return "filled"
	case IsSelect(n):
		return "selected in"
	default:
		return "clicked"
	}
}

// Criterion is one option offered to the picker.
type Criterion struct {
	Key  string
	Desc string
}

// CriteriaOptions tunes how candidates become picker options.
type CriteriaOptions struct {
	MaxCandidates int // above this the page is grouped (default 60)
	Goal          string
	Seen          map[string]int
	URL           string
}

// Criteria is the option list for one pick, plus the groups behind "g:" keys.
type Criteria struct {
	Options []Criterion
	Groups  map[string][]*Candidate // "g:<anchor id>" -> members; nil when not grouped
}

// Keys returns the option keys in order.
func (c Criteria) Keys() []string {
	keys := make([]string, len(c.Options))
	for i, o := range c.Options {
		keys[i] = o.Key
	}
	return keys
}

// Has reports whether key is one of the options.
func (c Criteria) Has(key string) bool {
	for _, o := range c.Options {
		if o.Key == key {
			return true
		}
	}
	return false
}

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("the a an to of and or for in on it its then go open page click find with from that this as is be at by so do up into get see use using again back list full book books category user") {
		stopWords[w] = true
	}
}

var nonAlnumSpace = regexp.MustCompile(`[^a-z0-9 ]+`)
var nonAlnumRun = regexp.MustCompile(`[^a-z0-9]+`)

// GoalTokens returns the goal's content words, lowercased.
func GoalTokens(goal string) []string {
	clean := nonAlnumSpace.ReplaceAllString(strings.ToLower(goal), " ")
	var out []string
	for _, w := range strings.Fields(clean) {
		numeric := w[0] >= '0' && w[0] <= '9'
		if (len(w) >= 2 || numeric) && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// goalHits counts goal tokens found in the candidate's name, href, component, and props.
// Tokens shorter than three characters must match a whole word.
func goalHits(c *Candidate, tokens []string) int {
	hay := " " + strings.ToLower(c.Node.Name+" "+c.Node.Attrs["href"]+" "+c.Node.Comp+" "+strings.Join(propValues(c.Node), " ")) + " "
	hay = nonAlnumRun.ReplaceAllString(hay, " ")
	hits := 0
	for _, t := range tokens {
		if len(t) >= 3 {
			if strings.Contains(hay, t) {
				hits++
			}
		} else if strings.Contains(hay, " "+t+" ") {
			hits++
		}
	}
	return hits
}

// BuildCriteria turns candidates into picker options. Small pages list every
// element. Large pages promote up to 20 goal-relevant elements and fold the rest
// into one option per owning component or landmark ("g:<id>"), which the loop
// expands with a second pick. Meta actions are appended, subject to the repeat
// guard; scroll is offered only when some candidate is outside the viewport.
func BuildCriteria(cands []*Candidate, opts CriteriaOptions) Criteria {
	maxN := opts.MaxCandidates
	if maxN <= 0 {
		maxN = 60
	}
	var crit Criteria
	if len(cands) > maxN {
		tokens := GoalTokens(opts.Goal)
		type scored struct {
			c    *Candidate
			hits int
		}
		var relevant []scored
		for _, c := range cands {
			if h := goalHits(c, tokens); h > 0 {
				relevant = append(relevant, scored{c, h})
			}
		}
		sort.SliceStable(relevant, func(i, j int) bool { return relevant[i].hits > relevant[j].hits })
		promoted := map[string]bool{}
		for i, r := range relevant {
			if i >= 20 {
				break
			}
			promoted[r.c.Key] = true
		}
		crit.Groups = map[string][]*Candidate{}
		labels := map[string]string{}
		var order []string
		for _, c := range cands {
			if promoted[c.Key] {
				crit.Options = append(crit.Options, Criterion{c.Key, c.Desc})
				continue
			}
			anchor := c.Node.ParentComp
			if anchor == nil {
				anchor = c.Node.Landmark
			}
			gid := "g:page"
			label := "rest of page"
			if anchor != nil {
				gid = "g:" + anchor.ID
				switch {
				case anchor.Comp != "":
					label = CompLabel(anchor)
				case anchor.Name != "":
					label = fmt.Sprintf("%s %q", anchor.Role, trunc(anchor.Name, 30))
				default:
					label = anchor.Role
				}
			}
			if _, ok := crit.Groups[gid]; !ok {
				order = append(order, gid)
				labels[gid] = label
			}
			crit.Groups[gid] = append(crit.Groups[gid], c)
		}
		for _, gid := range order {
			members := crit.Groups[gid]
			var sample []string
			for i, m := range members {
				if i >= 5 {
					sample = append(sample, "…")
					break
				}
				s := m.Node.Name
				if s == "" {
					s = m.Desc
				}
				sample = append(sample, trunc(s, 28))
			}
			crit.Options = append(crit.Options, Criterion{gid, fmt.Sprintf("%s with %d elements: %s", labels[gid], len(members), strings.Join(sample, " · "))})
		}
	} else {
		for _, c := range cands {
			crit.Options = append(crit.Options, Criterion{c.Key, c.Desc})
		}
	}
	offViewport := false
	for _, c := range cands {
		if !c.Node.InViewport {
			offViewport = true
			break
		}
	}
	for _, k := range []string{MetaBack, MetaScroll} {
		if k == MetaScroll && !offViewport {
			continue
		}
		if opts.Seen[opts.URL+"|"+k] >= 2 {
			continue
		}
		crit.Options = append(crit.Options, Criterion{k, metaDesc[k]})
	}
	return crit
}

// GroupCriteria lists one group's members as options for the second pick.
func GroupCriteria(members []*Candidate) Criteria {
	var crit Criteria
	for i, c := range members {
		if i >= 80 {
			break
		}
		crit.Options = append(crit.Options, Criterion{c.Key, c.Desc})
	}
	return crit
}
