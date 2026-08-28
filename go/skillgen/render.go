package skillgen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/sightmap/sightmap/go/sightmap"
)

// beginRegion/endRegion delimit one NAMED managed region of a generated file.
// Content between a matching begin/end pair is regenerated on every run;
// everything else — the H1 and summary blockquote, the "How to get to it" and
// "Gotchas" prose an area file sandwiches between its two regions — is
// author-owned: seeded once when the file doesn't yet exist, and never
// touched again. Regions are named (not positional) so reconcile can match
// them up even if unrelated static text around them changes. See reconcile.go.
func beginRegion(name string) string { return fmt.Sprintf("<!-- sightmap:begin %s -->", name) }
func endRegion(name string) string   { return fmt.Sprintf("<!-- sightmap:end %s -->", name) }

// File is one file the generator produces, path relative to the skill
// directory root (e.g. "SKILL.md", "references/areas/library-ui.md"). Content
// is the fresh, full render — as if this were the first generation; Write and
// CheckTree are what reconcile it against anything already on disk.
type File struct {
	Path    string
	Content []byte
}

// Render renders every file a Router produces, as a pure function of the
// corpus alone: the SKILL.md router, the references/README.md index (using
// every area's freshly DERIVED summary, since there is nothing on disk yet to
// read back), and one references/areas/<slug>.md per area. This is the right
// call for a from-scratch generation and for anything (tests, golden output)
// that must not depend on filesystem state. A generate against an existing
// tree should use Files instead, so a hand-edited area summary reaches the
// index — see Files' doc comment.
func Render(r *Router) ([]File, error) {
	files := make([]File, 0, len(r.Areas)+2)
	files = append(files, File{Path: "SKILL.md", Content: renderSkill(r)})
	files = append(files, File{Path: "references/README.md", Content: renderIndex(r, nil)})
	for _, a := range r.Areas {
		files = append(files, File{
			Path:    "references/areas/" + a.Slug + ".md",
			Content: renderArea(a),
		})
	}
	return files, nil
}

// renderSkill renders the router SKILL.md. It has no managed-region markers:
// the whole file is generated scaffolding, with nothing corpus-specific for
// an author to hand-edit that isn't better edited in an area file instead.
func renderSkill(r *Router) []byte {
	var b bytes.Buffer
	project := r.Opts.AppTitle
	if project == "" {
		project = r.Opts.SkillName
	}

	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", r.Opts.SkillName)
	fmt.Fprintf(&b, "description: %s\n", yamlQuote(RouterDescription(project, r.Areas, r.Opts.DescriptionBudget)))
	fmt.Fprintf(&b, "---\n\n")

	fmt.Fprintf(&b, "# Drive %s\n\n", project)
	fmt.Fprintf(&b, "This app has a sightmap corpus at `.sightmap/`: %d area%s, %d view%s, %d component%s with stable names. Read the area file for whatever the request names before opening a browser — it has the routes, the component names, and the exact commands. Do not guess selectors; the corpus already has them.\n\n",
		len(r.Areas), plural(len(r.Areas)),
		totalViews(r.Areas), plural(totalViews(r.Areas)),
		totalComponents(r.Areas), plural(totalComponents(r.Areas)))

	fmt.Fprintf(&b, "## Pick a flow\n\n")
	fmt.Fprintf(&b, "Read [references/README.md](references/README.md) for the full area index. Each area's reference file names the routes, components, and vocabulary for that part of the app.\n\n")

	fmt.Fprintf(&b, "## Quick start\n\n")
	fmt.Fprintf(&b, "```bash\n")
	fmt.Fprintf(&b, "sightmap version                     # is the CLI installed?\n")
	fmt.Fprintf(&b, "sightmap browser start                # launch Chrome + the overlay server\n")
	fmt.Fprintf(&b, "sightmap browser status               # current URL\n")
	fmt.Fprintf(&b, "sightmap snapshot --coverage           # read the page as an annotated component tree\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## Command surface\n\n")
	fmt.Fprintf(&b, "Categories, not an exhaustive list. **`--help` is canonical** — run `sightmap <command> --help` before inventing a flag.\n\n")
	fmt.Fprintf(&b, "| Category | Commands |\n")
	fmt.Fprintf(&b, "|---|---|\n")
	fmt.Fprintf(&b, "| Session | `browser start` · `stop` · `status` · `navigate '<url>'` (positional, no `--url`) · `tabs list\\|new\\|close\\|resize` |\n")
	fmt.Fprintf(&b, "| Observe | `snapshot [--coverage]` · `browser bounds` · `browser eval 'js'` |\n")
	fmt.Fprintf(&b, "| Act | `browser click` · `fill [--clear]` · `hover` · `keypress` · `scroll` · `drag` · `dialog accept\\|dismiss` |\n")
	fmt.Fprintf(&b, "| Synchronize | `browser wait-for --view NAME \\| --component 'Query' \\| --selector CSS \\| --url SUBSTR \\| --load` |\n")
	fmt.Fprintf(&b, "| Evidence | `browser screenshot --out F.png [--component NAME] [--expand-pct N]` |\n")
	fmt.Fprintf(&b, "| Debug | `console list\\|get` · `network list\\|get` |\n\n")
	fmt.Fprintf(&b, "Every verb accepts `--addr`, `--tab`, `--sightmap-dir`.\n\n")

	fmt.Fprintf(&b, "## Proof bar\n\n")
	fmt.Fprintf(&b, "A claim that something works is not evidence. Before saying a flow passes:\n\n")
	fmt.Fprintf(&b, "1. `sightmap browser wait-for --view <ViewName>` succeeded — you are demonstrably on the right route, not a redirect or an error page.\n")
	fmt.Fprintf(&b, "2. `sightmap snapshot --coverage` returned a non-empty tree. Zero interactive nodes renders `∅` and exits non-zero: the page is blank or still hydrating.\n")
	fmt.Fprintf(&b, "3. A screenshot for anything visual, clipped to the component under test with `--component`.\n")
	fmt.Fprintf(&b, "4. If a step was skipped, say which one and why — don't silently narrow scope.\n\n")

	fmt.Fprintf(&b, "## Driving conventions\n\n")
	fmt.Fprintf(&b, "- **Address by component query, not by probe ID.** IDs come from one snapshot and go stale the moment the page re-renders. Queries re-resolve atomically.\n")
	fmt.Fprintf(&b, "- **Whitespace is a descendant combinator** (`LibraryTable LibrarySearchBar`). There is no `>` child combinator.\n")
	fmt.Fprintf(&b, "- **Predicates**: `Name[prop=value]`, ops `=`, `^=` (prefix), `*=` (substring), trailing ` i` for case-insensitive.\n")
	fmt.Fprintf(&b, "- **Occurrence**: `Name#N`, 0-based, when several nodes match.\n")
	fmt.Fprintf(&b, "- **After any navigating action**, `wait-for --view` before the next snapshot.\n\n")

	fmt.Fprintf(&b, "## Feature map\n\n")
	fmt.Fprintf(&b, "%d areas, one reference file each, under [`references/`](references/README.md).\n", len(r.Areas))

	return normalize(b.Bytes())
}

// renderIndex renders references/README.md: the area map, grouped and linked,
// each with its one-line summary. README.md carries no managed-region
// markers of its own — it is entirely derived, so it is fully regenerated
// every run. summaries, keyed by Area.Slug, is the ACTUAL on-disk summary for
// an area whose file already exists (read back by Files, so a hand-edit in
// the area file's blockquote reaches the index); a missing entry (nil map,
// or an area with no file yet) falls back to the derived areaSummary — this
// is what a first generation, with nothing on disk to read, uses for every
// area.
func renderIndex(r *Router, summaries map[string]string) []byte {
	var b bytes.Buffer

	fmt.Fprintf(&b, "# Feature map — %s\n\n", nonEmpty(r.Opts.AppTitle, r.Opts.SkillName))

	fmt.Fprintf(&b, "## Baseline preconditions\n\n")
	fmt.Fprintf(&b, "- `sightmap version` succeeds (else `npm install -g @sightmap/sightmap`).\n")
	fmt.Fprintf(&b, "- `sightmap browser start`, then `sightmap browser status` shows a real URL.\n")
	fmt.Fprintf(&b, "- Prefer component queries over raw CSS selectors; see the area file for this page.\n\n")

	fmt.Fprintf(&b, "## Proof and skip reporting\n\n")
	fmt.Fprintf(&b, "Report per area: PASS with the snapshot/screenshot that proves it, FAIL with the command and its output, or SKIP with the precondition that could not be met.\n\n")

	fmt.Fprintf(&b, "## Full sweep\n\n")
	fmt.Fprintf(&b, "Walk the areas below top to bottom for a broad regression. Read the area file before touching the browser — it has the routes, components, and commands.\n\n")

	fmt.Fprintf(&b, "## Areas\n\n")
	for _, a := range r.Areas {
		summary := summaries[a.Slug]
		if summary == "" {
			summary = areaSummary(a)
		}
		fmt.Fprintf(&b, "- [%s](areas/%s.md): %s\n", a.Slug, a.Slug, summary)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Entry contract\n\n")
	fmt.Fprintf(&b, "Every area file above uses exactly these four H2s, in this order:\n\n")
	fmt.Fprintf(&b, "1. `## Sub-features` — every named view and component in the area, with the natural-language phrases that map a prompt onto it.\n")
	fmt.Fprintf(&b, "2. `## How to get to it (user POV)` — routes and the words a user would use.\n")
	fmt.Fprintf(&b, "3. `## Driving it with sightmap browser` — a runnable `sightmap browser` block, then the query forms for this area.\n")
	fmt.Fprintf(&b, "4. `## Gotchas` — quirks not recoverable from the DOM.\n\n")
	fmt.Fprintf(&b, "An area file with fewer than four H2s is stale; run `sightmap skills generate`.\n")

	return normalize(b.Bytes())
}

// renderArea renders one area's reference file: the four-H2 entry contract in
// order (Sub-features, How to get to it, Driving it, Gotchas — see
// references/README.md's "Entry contract"). The two corpus-derived sections
// are each their own named managed region; the H1/summary above them and the
// "How to get to it"/"Gotchas" prose sandwiched between and after them are
// author-owned, seeded once on first generation and never touched again. See
// beginRegion/endRegion and reconcile.go.
func renderArea(a Area) []byte {
	var b bytes.Buffer

	fmt.Fprintf(&b, "# %s\n\n", a.Title)
	fmt.Fprintf(&b, "> %s\n\n", areaSummary(a))

	fmt.Fprintf(&b, "%s\n", beginRegion("sub-features"))
	renderSubFeatures(&b, a)
	fmt.Fprintf(&b, "%s\n\n", endRegion("sub-features"))

	fmt.Fprintf(&b, "## How to get to it (user POV)\n\n")
	fmt.Fprintf(&b, "%s\n\n", seedUserPOV(a))

	fmt.Fprintf(&b, "%s\n", beginRegion("driving"))
	renderDriving(&b, a)
	fmt.Fprintf(&b, "%s\n\n", endRegion("driving"))

	fmt.Fprintf(&b, "## Gotchas\n\n")
	fmt.Fprintf(&b, "%s\n", seedGotchas())

	return normalize(b.Bytes())
}

func renderSubFeatures(b *bytes.Buffer, a Area) {
	fmt.Fprintf(b, "## Sub-features\n\n")
	lead := AreaLeadWord(a.Title)

	if len(a.Views) > 0 {
		fmt.Fprintf(b, "Views:\n\n")
		for _, v := range a.Views {
			renderViewBullet(b, v, lead)
		}
		fmt.Fprintf(b, "\n")
	}

	if len(a.Components) > 0 {
		fmt.Fprintf(b, "Components (available on every view in this area):\n\n")
		renderComponentTree(b, a.Components, lead)
		fmt.Fprintf(b, "\n")
	}

	if len(a.Requests) > 0 {
		fmt.Fprintf(b, "Requests:\n\n")
		for _, req := range a.Requests {
			renderRequestBullet(b, req)
		}
		fmt.Fprintf(b, "\n")
	}

	if len(a.Messages) > 0 {
		fmt.Fprintf(b, "Console/exception patterns:\n\n")
		for _, m := range a.Messages {
			renderMessageBullet(b, m)
		}
		fmt.Fprintf(b, "\n")
	}
}

func renderViewBullet(b *bytes.Buffer, v sightmap.ViewDef, lead string) {
	aliases := Aliases(v.Name, lead)
	fmt.Fprintf(b, "- `%s` — %s", v.Name, quotedList(aliases))
	if v.Route != "" {
		fmt.Fprintf(b, " · route `%s`", v.Route)
	}
	if v.Description != "" {
		fmt.Fprintf(b, ": %s", v.Description)
	}
	fmt.Fprintf(b, "\n")
	if !v.Access.IsOpen() {
		reason := v.Access.Reason
		if reason == "" {
			reason = v.Access.Status
		}
		fmt.Fprintf(b, "  - Not reachable by the reference account: %s\n", reason)
	}
	if v.Stability == "stub" || v.Stability == "deferred" {
		fmt.Fprintf(b, "  - Stability: %s\n", v.Stability)
	}
	renderComponentTree(b, v.Components, lead)
}

// renderComponentTree walks a flat, pre-order ComponentDef list (parent
// immediately before its flattened children — see sightmap's flattenAll) and
// indents by len(ParentChain), reproducing the authored hierarchy without
// rebuilding a tree.
func renderComponentTree(b *bytes.Buffer, comps []sightmap.ComponentDef, lead string) {
	for _, c := range comps {
		indent := strings.Repeat("  ", len(c.ParentChain)+1)
		aliases := Aliases(c.Name, lead)
		fmt.Fprintf(b, "%s- `%s` — %s", indent, c.Name, quotedList(aliases))
		if c.Description != "" {
			fmt.Fprintf(b, ": %s", c.Description)
		}
		if c.Stability == "uncertain" || c.Stability == "unstable" {
			fmt.Fprintf(b, " (selector stability: %s)", c.Stability)
		}
		fmt.Fprintf(b, "\n")
	}
}

func renderRequestBullet(b *bytes.Buffer, r sightmap.RequestDef) {
	fmt.Fprintf(b, "- `%s` — %s %s", r.Name, nonEmpty(r.Method, "ANY"), r.Route)
	if r.Description != "" {
		fmt.Fprintf(b, ": %s", r.Description)
	}
	fmt.Fprintf(b, "\n")
}

func renderMessageBullet(b *bytes.Buffer, m sightmap.MessageDef) {
	fmt.Fprintf(b, "- `%s`", m.Name)
	if m.Level != "" {
		fmt.Fprintf(b, " (level=%s)", m.Level)
	}
	if m.Description != "" {
		fmt.Fprintf(b, ": %s", m.Description)
	}
	fmt.Fprintf(b, "\n")
}

func renderDriving(b *bytes.Buffer, a Area) {
	fmt.Fprintf(b, "## Driving it with sightmap browser\n\n")

	fmt.Fprintf(b, "```bash\n")
	if len(a.Views) > 0 && a.Views[0].Route != "" {
		fmt.Fprintf(b, "sightmap browser navigate '<url matching %s>'\n", a.Views[0].Route)
		fmt.Fprintf(b, "sightmap browser wait-for --view %s\n", a.Views[0].Name)
	}
	fmt.Fprintf(b, "sightmap snapshot --coverage\n")
	printed := 0
	for _, c := range a.Components {
		if printed >= 3 {
			break
		}
		fmt.Fprintf(b, "%s\n", Command(c))
		printed++
	}
	fmt.Fprintf(b, "```\n\n")

	if len(a.Requests) > 0 {
		fmt.Fprintf(b, "```bash\n")
		fmt.Fprintf(b, "sightmap network list   # matched requests lead with [MatchedName]\n")
		fmt.Fprintf(b, "```\n\n")
	}

	fmt.Fprintf(b, "Address components by **query** (shown above), never by a raw CSS selector or a probe ID from an earlier snapshot.\n\n")

	if len(a.Memory) > 0 {
		fmt.Fprintf(b, "Author notes (`memory:`):\n\n")
		for _, m := range a.Memory {
			fmt.Fprintf(b, "- %s\n", m)
		}
		fmt.Fprintf(b, "\n")
	}
}

// seedUserPOV produces the first-generation content for the "How to get to
// it" section. It is a seed, not a verdict: written once, then left alone —
// an author corrects it in place and the fix survives every later run.
func seedUserPOV(a Area) string {
	if len(a.Views) == 0 {
		return "_Not yet described — this area has no pages of its own; its components appear inside other areas' views._"
	}
	var routes []string
	for _, v := range a.Views {
		if v.Route != "" {
			routes = append(routes, fmt.Sprintf("`%s`", v.Route))
		}
	}
	return fmt.Sprintf("_Not yet described. Routes in this area: %s._", strings.Join(routes, ", "))
}

// seedGotchas is the first-generation seed for the "Gotchas" section. memory:
// entries already render in the managed "Driving it" section as "Author
// notes", so there is no corpus signal left to seed this from yet.
func seedGotchas() string {
	return "_None recorded yet._"
}

// areaSummary is the cascade used for both the area file's own blockquote
// (on first generation) and the index line that reads it back: a sole view's
// description first, else a factual roll-up, else the title. This is the
// intentionally weak link the plan calls out — the corpus has no
// file-level description field, so a first generation can only guess; an
// author corrects it once, in the area file, and the fix round-trips into
// the index on the next run because reconcile preserves the blockquote and
// renderIndex is built from a.
func areaSummary(a Area) string {
	if len(a.Views) == 1 && a.Views[0].Description != "" {
		return a.Views[0].Description
	}
	parts := []string{}
	if n := len(a.Views); n > 0 {
		parts = append(parts, fmt.Sprintf("%d view%s", n, plural(n)))
	}
	if n := len(a.Components); n > 0 {
		parts = append(parts, fmt.Sprintf("%d component%s", n, plural(n)))
	}
	if n := len(a.Requests); n > 0 {
		parts = append(parts, fmt.Sprintf("%d request%s", n, plural(n)))
	}
	if len(parts) == 0 {
		return a.Title + "."
	}
	return strings.Join(parts, ", ") + "."
}

func totalViews(areas []Area) int {
	n := 0
	for _, a := range areas {
		n += len(a.Views)
	}
	return n
}

func totalComponents(areas []Area) int {
	n := 0
	for _, a := range areas {
		n += len(a.Components)
	}
	return n
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func quotedList(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}

// yamlQuote renders s as a double-quoted YAML scalar, escaping the two
// characters that matter inside one ("\\" and "\""). The frontmatter
// description is generated prose, never containing YAML-significant newlines,
// so this covers every real input without pulling in a YAML encoder.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// normalize enforces the wire-level determinism rules Check depends on:
// right-trim every line, collapse to a single trailing newline, LF only.
func normalize(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	out := strings.Join(lines, "\n")
	out = strings.TrimRight(out, "\n") + "\n"
	return []byte(out)
}
