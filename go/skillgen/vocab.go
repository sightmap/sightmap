package skillgen

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sightmap/sightmap/go/sightmap"
)

// initialisms are area-slug words rendered fully uppercase in a title instead
// of Title-cased ("dqm-ui" -> "DQM UI", not "Dqm UI"). Extend this list as new
// corpora need it; it is deliberately small rather than exhaustive.
var initialisms = map[string]bool{
	"ui": true, "api": true, "dqm": true, "url": true, "sdk": true,
	"ai": true, "qa": true, "css": true, "html": true, "js": true,
	"ts": true, "id": true, "sql": true, "csv": true, "dom": true,
}

// AreaTitle derives a human title from a corpus file slug: split on '-'/'_',
// title-case each word, and uppercase any word in the initialism list.
// "library-ui" -> "Library UI"; "dqm-ui" -> "DQM UI".
func AreaTitle(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if initialisms[strings.ToLower(p)] {
			parts[i] = strings.ToUpper(p)
			continue
		}
		if p == "" {
			continue
		}
		r, size := utf8.DecodeRuneInString(p)
		parts[i] = string(unicode.ToUpper(r)) + strings.ToLower(p[size:])
	}
	return strings.Join(parts, " ")
}

// AreaLeadWord returns the first word of a title ("Library UI" -> "Library"),
// used to derive a stripped alias for components whose name repeats the
// area's own name (LibrarySearchBar in library-ui -> also "search bar").
func AreaLeadWord(title string) string {
	fields := strings.Fields(title)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// splitWords splits a PascalCase/camelCase identifier into its constituent
// words, keeping acronym runs whole: "LibrarySearchBar" -> [Library Search
// Bar]; "DQMTable" -> [DQM Table]; "LibraryUI" -> [Library UI].
//
// A boundary is cut before rune i when: a lower/digit is followed by an
// upper (word start); an upper-upper pair is followed by a lower (the run of
// capitals was an acronym and rune i-1 starts the next word, e.g. "DQMTable"
// cuts before the T, not the M); or a letter/digit transition occurs. '-',
// '_', '.', and space are separators that never appear in the output.
func splitWords(name string) []string {
	if name == "" {
		return nil
	}
	runes := []rune(name)
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	for i, r := range runes {
		if r == '-' || r == '_' || r == '.' || r == ' ' {
			flush()
			continue
		}
		if i > 0 {
			prev := runes[i-1]
			switch {
			case (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(r):
				// word start: a lower/digit run ends, an upper run begins.
				// Covers ordinary PascalCase ("library|Table") and a
				// digit-then-word boundary ("h1|Heading").
				flush()
			case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
				// acronym boundary: a run of capitals hands off to a new word
				// ("DQ|MTable" would be wrong; this cuts "DQM|Table" instead,
				// keeping the acronym run whole and starting the new word at
				// the last capital).
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// Phrase renders a component/view name as the lowercase natural-language
// phrase a user would type: "LibrarySearchBar" -> "library search bar". No
// suffix stripping — the phrase's job is substring/token overlap with a
// user's own words, and aggressive stemming loses more than it gains.
func Phrase(name string) string {
	words := splitWords(name)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, " ")
}

// Aliases returns the phrases that should route a natural-language prompt to
// this component: always the full phrase, plus — when the name leads with
// the area's own lead word — the phrase with that leading word stripped.
// "LibrarySearchBar" in area "library-ui" (lead word "Library") yields
// ["library search bar", "search bar"], so "click the search bar" resolves.
func Aliases(componentName, areaLeadWord string) []string {
	full := Phrase(componentName)
	if full == "" {
		return nil
	}
	aliases := []string{full}
	lead := strings.ToLower(areaLeadWord)
	if lead == "" {
		return aliases
	}
	prefix := lead + " "
	if stripped, ok := strings.CutPrefix(full, prefix); ok && stripped != "" {
		aliases = append(aliases, stripped)
	}
	return aliases
}

// Verb keys VerbFor can return. Each has its own argument shape (see
// Command), so callers must not treat the result as a literal CLI fragment.
const (
	VerbClick   = "click"
	VerbFill    = "fill"
	VerbWaitFor = "wait-for"
	VerbHover   = "hover"
)

// verbRules maps a component's name suffix to the sightmap browser verb that
// most likely drives it. Checked in order; the first matching suffix wins.
// A name matching no rule falls back to VerbHover: every `browser` target
// accepts it, and it never mutates state, so it's always a safe suggestion.
var verbRules = []struct{ suffix, verb string }{
	{"MenuActions", VerbClick},
	{"Button", VerbClick},
	{"Link", VerbClick},
	{"Tab", VerbClick},
	{"SearchBar", VerbFill},
	{"Input", VerbFill},
	{"Field", VerbFill},
	{"Picker", VerbFill},
	{"Modal", VerbWaitFor},
	{"Pane", VerbWaitFor},
	{"Table", VerbWaitFor},
	{"List", VerbWaitFor},
	{"Container", VerbWaitFor},
}

// VerbFor returns the sightmap browser verb most likely to drive a component,
// derived from a fixed suffix table on its name. Render the result with
// Command, not string concatenation — click/hover/wait-for take a single
// query argument, but fill also needs a value and wait-for needs `--component`.
func VerbFor(name string) string {
	for _, r := range verbRules {
		if strings.HasSuffix(name, r.suffix) {
			return r.verb
		}
	}
	return VerbHover
}

// Command renders the full `sightmap browser ...` invocation suggested for
// driving c, using VerbFor's verb and Query's address. Centralizing this
// (rather than letting a caller paste verb+query together) is what keeps
// `fill`'s required value argument and `wait-for`'s `--component` flag from
// being silently dropped.
func Command(c sightmap.ComponentDef) string {
	query := Query(c)
	switch VerbFor(c.Name) {
	case VerbFill:
		return fmt.Sprintf(`sightmap browser fill --clear '%s' "…"`, query)
	case VerbWaitFor:
		return fmt.Sprintf(`sightmap browser wait-for --component '%s'`, query)
	case VerbClick:
		return fmt.Sprintf(`sightmap browser click '%s'`, query)
	default:
		return fmt.Sprintf(`sightmap browser hover '%s'`, query)
	}
}

// Query renders the sightmap component-query address for c, by precedence:
// a component with declared properties addresses by its first property
// (values are read off the live page, so the value is a placeholder); a
// nested component addresses by its full descendant chain; anything else
// addresses by its bare name.
func Query(c sightmap.ComponentDef) string {
	if len(c.Properties) > 0 {
		return fmt.Sprintf(`%s[%s="…"]`, c.Name, c.Properties[0].Name)
	}
	if len(c.ParentChain) > 0 {
		parts := append(append([]string{}, c.ParentChain...), c.Name)
		return strings.Join(parts, " ")
	}
	return c.Name
}

// weight is an area's ranking priority for description truncation: more
// declared entities means more likely to matter to a cold prompt.
func weight(a Area) int {
	return len(a.Views) + len(a.Components) + len(a.Requests) + len(a.Messages)
}

// orderedByWeight returns areas sorted by descending weight, tie-broken by
// slug ascending — a stable, deterministic priority order for anything that
// must degrade gracefully under a length budget.
func orderedByWeight(areas []Area) []Area {
	ordered := append([]Area(nil), areas...)
	sort.SliceStable(ordered, func(i, j int) bool {
		wi, wj := weight(ordered[i]), weight(ordered[j])
		if wi != wj {
			return wi > wj
		}
		return ordered[i].Slug < ordered[j].Slug
	})
	return ordered
}

// descriptionBudgetDefault is the default rune ceiling for the router
// frontmatter description — the only text visible to the model's
// auto-invoke index before this skill's body is read.
const descriptionBudgetDefault = 900

// RouterDescription builds the frontmatter `description` for the router
// skill: a fixed trigger sentence around a comma-separated list of area
// titles (lowercased), ordered by weight so the biggest surfaces survive
// truncation first. Never truncates mid-word: whole area titles are dropped
// from the tail and replaced with a counted "+N more" note. budget <= 0 uses
// descriptionBudgetDefault.
func RouterDescription(project string, areas []Area, budget int) string {
	if budget <= 0 {
		budget = descriptionBudgetDefault
	}
	head := fmt.Sprintf("Use when a request names part of the %s app UI — ", project)
	tail := " — or asks to click, fill, verify, or screenshot something in it. " +
		"Maps those words onto sightmap component queries and `sightmap browser` commands."

	ordered := orderedByWeight(areas)
	titles := make([]string, len(ordered))
	for i, a := range ordered {
		titles[i] = strings.ToLower(a.Title)
	}

	fits := func(n int) (string, bool) {
		kept := titles[:n]
		remaining := len(titles) - n
		suffix := ""
		if remaining > 0 {
			word := "areas"
			if remaining == 1 {
				word = "area"
			}
			suffix = fmt.Sprintf(", and %d more %s", remaining, word)
		}
		s := head + strings.Join(kept, ", ") + suffix + tail
		return s, utf8.RuneCountInString(s) <= budget
	}

	best := head + tail
	for n := len(titles); n >= 0; n-- {
		if s, ok := fits(n); ok {
			best = s
			break
		}
	}
	return best
}
