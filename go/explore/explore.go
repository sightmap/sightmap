package explore

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Options configures one goal.
type Options struct {
	Goal          string
	Spec          *Spec
	Picker        Picker
	MaxSteps      int     // default 20
	MaxCandidates int     // default 60
	DoneThreshold float64 // picker "done" confidence that ends a goal with no deterministic check (default 0.85)
	// Hook runs on every observed page before candidates are built (used by --grow).
	Hook PageHook
	// OnStep is called after each step with its record.
	OnStep func(Step)
}

// PageHook sees every page the loop observes.
type PageHook interface {
	OnPage(ctx context.Context, page *Page) error
}

// Step records one iteration.
type Step struct {
	N         int      `json:"step"`
	URL       string   `json:"url"`
	View      string   `json:"view,omitempty"`
	Coverage  *CovStat `json:"coverage,omitempty"`
	Pick      string   `json:"pick,omitempty"`
	Group     string   `json:"group,omitempty"`
	Probs     string   `json:"probs,omitempty"`
	DoneProb  float64  `json:"done_prob"`
	Why       string   `json:"why,omitempty"`
	Action    string   `json:"action"`
	URLAfter  string   `json:"url_after,omitempty"`
	MsSnap    int      `json:"ms_snap"`
	MsPick    int      `json:"ms_pick"`
	MsAct     int      `json:"ms_act"`
	MsSettle  int      `json:"ms_settle"`
	Ms        int      `json:"ms"`
	Navigated bool     `json:"navigated,omitempty"`
}

// CovStat is the page's coverage at the moment of a step.
type CovStat struct {
	Interactive int `json:"interactive"`
	T1          int `json:"t1"`
	T2          int `json:"t2"`
	Orphaned    int `json:"t3"`
}

// Transition is one observed edge of the site graph.
type Transition struct {
	From    string `json:"from"`
	Action  string `json:"action"`
	Comp    string `json:"comp,omitempty"`
	To      string `json:"to"`
	Changed bool   `json:"changed"`
}

// Run is the result of one goal.
type Run struct {
	Goal        string       `json:"goal"`
	Spec        *Spec        `json:"spec,omitempty"`
	Picker      string       `json:"picker"`
	OK          bool         `json:"ok"`
	Reason      string       `json:"reason"`
	Steps       []Step       `json:"steps"`
	Transitions []Transition `json:"transitions"`
	Ms          int          `json:"ms"`
	Stats       Stats        `json:"picker_stats"`
	HookErrors  int          `json:"hook_errors,omitempty"`
}

// Explore drives the browser toward opts.Goal and returns the run. It returns
// an error only for failures outside the loop's control (a picker outage, a
// dead connection); a goal that is not reached is a Run with OK=false.
func Explore(ctx context.Context, drv Driver, opts Options) (*Run, error) {
	if opts.Picker == nil {
		return nil, fmt.Errorf("explore: no picker")
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 20
	}
	if opts.DoneThreshold <= 0 {
		opts.DoneThreshold = 0.85
	}
	spec := opts.Spec
	if spec == nil {
		spec = &Spec{}
	}
	values := spec.Values
	usedValues := map[string]bool{}
	seen := map[string]int{}
	var history []string
	run := &Run{Goal: opts.Goal, Spec: spec, Picker: opts.Picker.Name()}
	t0 := time.Now()
	defer func() {
		run.Ms = int(time.Since(t0).Milliseconds())
		run.Stats = opts.Picker.Stats()
	}()

	for n := 1; n <= opts.MaxSteps; n++ {
		if ctx.Err() != nil {
			run.Reason = "cancelled"
			return run, nil
		}
		tS := time.Now()
		page, err := drv.Observe(ctx)
		if err != nil {
			return run, fmt.Errorf("explore: observe: %w", err)
		}
		if len(page.Nodes) < 3 {
			drv.Settle(ctx, page.URL)
			if page, err = drv.Observe(ctx); err != nil {
				return run, fmt.Errorf("explore: observe: %w", err)
			}
		}
		step := Step{N: n, URL: page.URL, View: page.View, Coverage: covStat(page), MsSnap: int(time.Since(tS).Milliseconds())}
		if len(run.Transitions) > 0 {
			run.Transitions[len(run.Transitions)-1].To = pageLabel(page)
		}
		if opts.Hook != nil {
			if err := opts.Hook.OnPage(ctx, page); err != nil {
				run.HookErrors++
			}
		}

		if spec.DoneWhen.Deterministic() && spec.DoneWhen.Check(page, history) {
			step.Action = "done"
			run.Steps = append(run.Steps, step)
			run.OK = true
			run.Reason = "done_when satisfied"
			emit(opts, step)
			return run, nil
		}

		cands := Candidates(page.Nodes, CandidateOptions{Seen: seen, URL: page.URL, Avoid: spec.Avoid})
		if len(cands) == 0 && len(Candidates(page.Nodes, CandidateOptions{Avoid: spec.Avoid})) > 0 {
			// Every control here has already been tried twice; forget this page's history once and try again.
			for k := range seen {
				if strings.HasPrefix(k, page.URL+"|") {
					delete(seen, k)
				}
			}
			cands = Candidates(page.Nodes, CandidateOptions{Seen: seen, URL: page.URL, Avoid: spec.Avoid})
		}
		if len(cands) == 0 {
			step.Action = "no-candidates"
			run.Steps = append(run.Steps, step)
			run.Reason = "no actionable elements"
			emit(opts, step)
			return run, nil
		}
		crit := BuildCriteria(cands, CriteriaOptions{MaxCandidates: opts.MaxCandidates, Goal: opts.Goal, Seen: seen, URL: page.URL})
		state := buildState(opts.Goal, spec, page, history, cands)

		tP := time.Now()
		pick, err := opts.Picker.Pick(ctx, state, crit)
		if err != nil {
			return run, fmt.Errorf("explore: pick: %w", err)
		}
		if strings.HasPrefix(pick.Next, "g:") && crit.Groups != nil {
			members := crit.Groups[pick.Next]
			second, err := opts.Picker.Pick(ctx, state, GroupCriteria(members))
			if err != nil {
				return run, fmt.Errorf("explore: pick in group: %w", err)
			}
			step.Group = pick.Next
			if second.Done < pick.Done {
				second.Done = pick.Done
			}
			pick = second
		}
		step.MsPick = int(time.Since(tP).Milliseconds())
		step.Pick = pick.Next
		step.Probs = topProbs(pick.Probs, 3)
		step.DoneProb = pick.Done
		step.Why = pick.Why

		if pick.Done >= opts.DoneThreshold && !spec.DoneWhen.Deterministic() {
			step.Action = "done(judged)"
			run.Steps = append(run.Steps, step)
			run.OK = true
			run.Reason = fmt.Sprintf("picker judged done (%.2f)", pick.Done)
			emit(opts, step)
			return run, nil
		}

		tA := time.Now()
		act, err := perform(ctx, drv, opts.Picker, pick.Next, cands, values, usedValues, state, page)
		if err != nil && isStale(err) {
			// The page re-rendered between snapshot and act: re-observe and retry the same element by description.
			fresh, oErr := drv.Observe(ctx)
			if oErr != nil {
				return run, fmt.Errorf("explore: observe: %w", oErr)
			}
			want := findCandidate(cands, pick.Next)
			var again *Node
			if want != nil {
				for _, fn := range fresh.Nodes {
					if fn.Interactive && fn.Visible && Describe(fn) == want.Desc {
						again = fn
						break
					}
				}
			}
			if again != nil {
				retry := []*Candidate{{Key: "n" + again.ID, Node: again, Desc: want.Desc, SeenKey: want.SeenKey}}
				act, err = perform(ctx, drv, opts.Picker, retry[0].Key, retry, values, usedValues, state, fresh)
			}
			if again == nil || (err != nil && isStale(err)) {
				// Gone twice: record the miss as a step and let the next observation decide.
				seenKey := pick.Next
				if want != nil {
					seenKey = want.SeenKey
				}
				act = &action{summary: fmt.Sprintf("stale element, skipped (%s)", pickLabel(want, pick.Next)), seenKey: seenKey, urlAfter: fresh.URL}
				err = nil
			}
		}
		if err != nil {
			return run, fmt.Errorf("explore: act: %w", err)
		}
		step.MsAct = int(time.Since(tA).Milliseconds()) - act.settleMs
		if step.MsAct < 0 {
			step.MsAct = 0
		}
		step.MsSettle = act.settleMs
		step.Action = act.summary
		step.URLAfter = act.urlAfter
		step.Navigated = act.urlAfter != page.URL
		step.Ms = int(time.Since(tS).Milliseconds())
		run.Steps = append(run.Steps, step)
		run.Transitions = append(run.Transitions, Transition{From: pageLabel(page), Action: act.summary, Comp: act.comp, To: shortURL(act.urlAfter), Changed: step.Navigated})
		history = append(history, fmt.Sprintf("%d. %s → %s", n, act.summary, shortURL(act.urlAfter)))
		seen[page.URL+"|"+act.seenKey]++
		emit(opts, step)
	}
	run.Reason = fmt.Sprintf("no result within %d steps", opts.MaxSteps)
	return run, nil
}

func emit(opts Options, s Step) {
	if opts.OnStep != nil {
		opts.OnStep(s)
	}
}

type action struct {
	summary  string
	comp     string
	seenKey  string
	urlAfter string
	settleMs int
}

func perform(ctx context.Context, drv Driver, picker Picker, pick string, cands []*Candidate, values map[string]string, usedValues map[string]bool, state string, page *Page) (*action, error) {
	act := &action{seenKey: pick}
	switch pick {
	case MetaBack:
		if err := drv.Back(ctx); err != nil {
			return nil, err
		}
		act.summary = "went back"
	case MetaScroll:
		if err := drv.Scroll(ctx); err != nil {
			return nil, err
		}
		act.summary = "scrolled down"
	default:
		c := findCandidate(cands, pick)
		if c == nil {
			return nil, fmt.Errorf("picked unknown key %q", pick)
		}
		n := c.Node
		act.comp = n.Comp
		act.seenKey = c.SeenKey
		label := CompLabel(n)
		if label == "" {
			label = fmt.Sprintf("%s %q", n.Role, trunc(n.Name, 40))
		}
		switch {
		case IsTextInput(n):
			key, val, ok, err := chooseValue(ctx, picker, n, values, usedValues, state)
			if err != nil {
				return nil, err
			}
			if ok {
				if err := drv.Fill(ctx, n, val); err != nil {
					return nil, err
				}
				usedValues[key] = true
				act.summary = fmt.Sprintf("filled %s with %s", label, key)
			} else {
				if err := drv.Click(ctx, n); err != nil {
					return nil, err
				}
				act.summary = fmt.Sprintf("focused %s (no value to type)", label)
			}
		case IsSelect(n):
			opts, err := drv.SelectOptions(ctx, n)
			if err != nil {
				return nil, err
			}
			idx, err := chooseOption(ctx, picker, n, opts, state)
			if err != nil {
				return nil, err
			}
			if err := drv.Select(ctx, n, idx); err != nil {
				return nil, err
			}
			chosen := ""
			if idx >= 0 && idx < len(opts) {
				chosen = opts[idx]
			}
			act.summary = fmt.Sprintf("selected %q in %s", chosen, label)
		case IsCheckable(n):
			if err := drv.Click(ctx, n); err != nil {
				return nil, err
			}
			act.summary = "toggled " + label
		default:
			if err := drv.Click(ctx, n); err != nil {
				return nil, err
			}
			act.summary = "clicked " + label
		}
	}
	info := drv.Settle(ctx, page.URL)
	act.settleMs = info.Ms
	act.urlAfter = info.URL
	if act.urlAfter == "" {
		if u, err := drv.URL(ctx); err == nil {
			act.urlAfter = u
		} else {
			act.urlAfter = page.URL
		}
	}
	return act, nil
}

func isStale(err error) bool {
	s := err.Error()
	return strings.Contains(s, "not found in live DOM") || strings.Contains(s, "not found") || strings.Contains(s, "noel")
}

func findCandidate(cands []*Candidate, key string) *Candidate {
	for _, c := range cands {
		if c.Key == key {
			return c
		}
	}
	return nil
}

func pickLabel(c *Candidate, key string) string {
	if c != nil {
		return c.Desc
	}
	return key
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// chooseValue picks which spec value to type into a field: by name overlap
// between the field and the value keys when that is unambiguous, otherwise by
// asking the picker. It returns ok=false when nothing fits.
func chooseValue(ctx context.Context, picker Picker, n *Node, values map[string]string, used map[string]bool, state string) (key, val string, ok bool, err error) {
	if len(values) == 0 {
		return "", "", false, nil
	}
	fieldWords := strings.ToLower(strings.Join([]string{n.Name, n.Attrs["placeholder"], n.Attrs["name"], n.Attrs["id"], n.Attrs["aria-label"], n.Attrs["type"], n.Comp}, " "))
	fieldWords = " " + nonAlnum.ReplaceAllString(fieldWords, " ") + " "
	type scored struct {
		key  string
		hits int
	}
	var best []scored
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		hits := 0
		for _, w := range strings.Fields(nonAlnum.ReplaceAllString(strings.ToLower(k), " ")) {
			if strings.Contains(fieldWords, w) {
				hits++
			}
		}
		if hits > 0 {
			best = append(best, scored{k, hits})
		}
	}
	if len(best) > 0 {
		sort.SliceStable(best, func(i, j int) bool {
			if best[i].hits != best[j].hits {
				return best[i].hits > best[j].hits
			}
			return !used[best[i].key] && used[best[j].key]
		})
		ties := 0
		for _, b := range best {
			if b.hits == best[0].hits {
				ties++
			}
		}
		if ties == 1 {
			return best[0].key, values[best[0].key], true, nil
		}
	}
	if len(keys) == 1 {
		return keys[0], values[keys[0]], true, nil
	}
	var crit Criteria
	for _, k := range keys {
		d := fmt.Sprintf("%s = %q", k, values[k])
		if used[k] {
			d += " (already typed once)"
		}
		crit.Options = append(crit.Options, Criterion{k, d})
	}
	crit.Options = append(crit.Options, Criterion{"none", "none of these belongs in this field"})
	res, err := picker.Choose(ctx, state+"\n\nFIELD TO FILL: "+Describe(n), crit, "Which value should be typed into this field?")
	if err != nil {
		return "", "", false, err
	}
	if res == "none" {
		return "", "", false, nil
	}
	if v, ok := values[res]; ok {
		return res, v, true, nil
	}
	return "", "", false, nil
}

func chooseOption(ctx context.Context, picker Picker, n *Node, opts []string, state string) (int, error) {
	if len(opts) <= 1 {
		return 0, nil
	}
	var crit Criteria
	for i, o := range opts {
		crit.Options = append(crit.Options, Criterion{fmt.Sprintf("o%d", i), o})
	}
	res, err := picker.Choose(ctx, state+"\n\nSELECT FIELD: "+Describe(n), crit, "Which option should be selected to move toward the goal?")
	if err != nil {
		return 0, err
	}
	var idx int
	if _, err := fmt.Sscanf(res, "o%d", &idx); err != nil || idx < 0 || idx >= len(opts) {
		return 0, nil
	}
	return idx, nil
}

// buildState renders the picker's context: goal, spec, page, recent steps,
// and every actionable element with its key.
func buildState(goal string, spec *Spec, page *Page, history []string, cands []*Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GOAL: %s\n", goal)
	if spec.Hint != "" {
		fmt.Fprintf(&b, "HINT: %s\n", spec.Hint)
	}
	fmt.Fprintf(&b, "DONE WHEN: %s\n", spec.DoneWhen.String())
	if len(spec.Values) > 0 {
		keys := make([]string, 0, len(spec.Values))
		for k := range spec.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var vals []string
		for _, k := range keys {
			vals = append(vals, fmt.Sprintf("%s=%q", k, spec.Values[k]))
		}
		fmt.Fprintf(&b, "VALUES YOU MAY TYPE: %s\n", strings.Join(vals, ", "))
	}
	view := page.View
	if view == "" {
		view = "unknown"
	}
	fmt.Fprintf(&b, "CURRENT PAGE: view=%s url=%s\n", view, shortURL(page.URL))
	if names := componentNames(page); len(names) > 0 {
		fmt.Fprintf(&b, "COMPONENTS ON PAGE: %s\n", strings.Join(names, ", "))
	}
	b.WriteString("RECENT STEPS:\n")
	if len(history) == 0 {
		b.WriteString("  (none yet)\n")
	}
	start := 0
	if len(history) > 8 {
		start = len(history) - 8
	}
	for _, h := range history[start:] {
		fmt.Fprintf(&b, "  %s\n", h)
	}
	b.WriteString("ACTIONABLE ELEMENTS (key: description):\n")
	for i, c := range cands {
		if i >= 200 {
			break
		}
		fmt.Fprintf(&b, "  %s: %s\n", c.Key, c.Desc)
	}
	fmt.Fprintf(&b, "  back: %s\n", metaDesc[MetaBack])
	b.WriteString("NOTE: every element on the page is already listed above, including ones below the fold; scrolling adds nothing unless the page loads more on scroll.\n")
	return b.String()
}

func componentNames(page *Page) []string {
	set := map[string]bool{}
	for _, n := range page.Nodes {
		if n.Comp != "" {
			set[n.Comp] = true
		}
	}
	names := make([]string, 0, len(set))
	for k := range set {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func covStat(page *Page) *CovStat {
	if page.Result == nil {
		return nil
	}
	cov := page.Result.Coverage
	return &CovStat{Interactive: cov.Total, T1: cov.T1, T2: cov.T2, Orphaned: cov.T3}
}

func pageLabel(page *Page) string {
	if page.View != "" {
		return page.View
	}
	return shortURL(page.URL)
}

func shortURL(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return u
	}
	s := p.Path
	if p.RawQuery != "" {
		s += "?" + p.RawQuery
	}
	if s == "" {
		s = "/"
	}
	return s
}
