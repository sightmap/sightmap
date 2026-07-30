# Failure-mode analysis: the whichbars authoring transcript

Source: [`transcripts/2026-07-29-whichbars.md`](transcripts/2026-07-29-whichbars.md)
(sightmap 0.15.9, macOS x64, agent-driven authoring run against
https://whichbars.com/). Root-cause references are to this repo at the time of
analysis.

## What happened, in one paragraph

An agent was asked to build a sightmap for a 4-page app, verify selectors with
`sel-probe`, and drive every page to 0 orphaned T3. It finished — validate
clean, lint clean, 0 orphaned on all four pages — and committed the corpus.
But the deliverable is structurally wrong: it contains **zero views**, because
the agent never discovered that view fields must be nested under a top-level
`views:` list. Roughly 60% of the session's ~110 tool calls were spent
guessing the view-file schema (including string-dumping the release binary to
read Go struct tags), and another ~25% working around `browser start` failures
by hand-launching Chrome and reverse-engineering the undocumented `.session`
file format. Every dead end was paved by a tool message that was silent,
generic, or actively misleading. The agent's closing note was accurate: *"the
tool accepted the fields without errors but didn't match pages to views."*

The individual failures compound: FM7 (docs) teaches the wrong shape → FM1
(loader) accepts it silently → FM2 (errors) gives contradictory correction →
FM3 (validate) certifies the workaround → the committed corpus permanently
loses views, capture sets, per-view reporting, and route-scoped semantics,
while cross-page selectors (`.table-wrap` as "AdminTable") silently annotate
the wrong pages.

## Failure modes

Each FM links to its eval case(s) under [`cases/`](cases/), which reproduce
the exact state and encode the desired behavior as assertions
(`known_failing: true` until the recommended PR lands).

### FM1 — The view-file schema trap (the session-killer)

**Observed.** The agent wrote `views/homepage.yaml` with `route: /` and
`components:` at the file root. `validate` said only
`warning: homepage.yaml: unknown field "route" (line 1)` and passed. Switching
to `url:` made the file validate **with no warnings at all** — because `url:`
is a legal *file-root* field (the file-level default URL for views) and
`components:` at the root legally defines *global* components. The file was
silently absorbed as a globals file; every snapshot printed
`[No view matched]` for the rest of the session; the agent eventually deleted
`views/` and shipped everything global.

**Root cause.**
- `go/sightmap/loader.go` (`loadDir`) routes any file's root `components:`
  into the global list and only creates views from a root `views:` sequence —
  there is no "this file looks like it wanted to be a view file" heuristic.
- `go/sightmap/unknownfields.go` knows `route` is a valid **view** field
  (`viewFields`) but reports it at file root with the same generic message as
  a typo, and accepts `url` at file root without comment (`fileRootFields`).
- `go/sightmap/validate.go` `missing-route` only fires for entries *inside* a
  `views:` list; a corpus with zero views raises nothing.

**Fix (PR-2).** Three targeted diagnostics: (1) a root-level key that is a
known view field gets a message showing the `views:` nesting with a two-line
example; (2) a file that has a root `url:` (or lives under `views/`) and
defines components but no views warns that it defines no views and its
components are global; (3) `validate` warns when the whole corpus has global
components but zero views.

**Evals.** `fm01-top-level-route-field`, `fm01-top-level-url-silent-globals`,
`fm01-zero-view-corpus-unflagged`, guarded by `ok-correct-view-file-validates`
and `ok-typo-field-warns`.

### FM2 — Contradictory error guidance (url vs route)

**Observed.** `report` failed with *"no views with URLs found / **Add a url:
field** to your views/\*.yaml files"* — while the agent's file already had a
root `url:`. Later, `capture` said *"...or **add a view with a matching
route**"*. The two messages name different fields and neither shows the
structure, which powered the `route:`/`url:`/`urls:`/`match:`/`pattern:`
guessing loop and convinced the agent the `route` warning "is just cosmetic".

**Root cause.** `go/cmd/sightmap/cmd_report.go:97-100` collapses "no views
defined at all" and "views exist but lack `url:`" into one message written for
the second case. `go/cmd/sightmap/cmd_capture.go:89` gives route-only advice.

**Fix (PR-3).** `report` distinguishes the two states; both commands print the
same 4-line `views:` example naming both fields' roles (`route:` matches,
`url:` navigates).

**Evals.** `fm02-report-error-teaches-schema`, guarded by
`ok-report-lists-views`.

### FM3 — Required `version:` field never enforced

**Observed.** No YAML written in the entire session contains `version: 1`.
`spec/v1/schema.md`: "Every file must begin with `version: 1`." Nothing warns.

**Root cause.** `rawFile.Version` is parsed (`go/sightmap/loader.go`) but
never checked.

**Fix (PR-2, same diagnostics pass).** Warn (not error, pre-1.0) when a corpus
file lacks `version:`.

**Evals.** `fm03-missing-version-not-flagged`.

### FM4 — `--help` exits 1 with "flag: help requested"

**Observed.** Every exploratory `--help` surfaced in the agent harness as
`Error: Exit code 1`, with trailing noise like
`sightmap validate: flag: help requested`. First contact with each subcommand
looked like a failure.

**Root cause.** Subcommands use `flag.ContinueOnError`; `fset.Parse` returns
`flag.ErrHelp`, and `go/cmd/sightmap/main.go:86-89` prints any non-nil error
and exits 1.

**Fix (PR-1).** In `main`, treat `errors.Is(err, flag.ErrHelp)` as success.
~5 lines.

**Evals.** `fm04-help-exits-nonzero`.

### FM5 — Session file: undocumented format + red-herring diagnostic

**Observed.** With no attach command (FM6), the agent hand-wrote
`.sightmap/.session`. `{"cdp_port":9222,"pid":33859}` parsed silently to
`Port: 0`, and `browser status` printed *"the sightmap server and CDP were
assigned the same port (0) — a known startup collision"* — a real hint for a
different bug, here a pure red herring. The agent brute-forced key names
(`CdpPort`, `cdp`, `port`) to find the format.

**Root cause.** `go/browser/launcher.go` `ReadSessionInfo` accepts any JSON
object without validating `Port > 0`; `go/cmd/sightmap/cmd_browser.go:270`
prints the collision hint whenever `ServerPort == Port`, including `0 == 0`.

**Fix (PR-4).** `ReadSessionInfo` errors on `Port <= 0` with the expected
shape (`{"port":N,"pid":N,...}`); the collision hint requires
`ServerPort > 0`; document `.session` in `docs/cli/browser.mdx`.

**Evals.** `fm05-session-file-red-herring`.

### FM6 — The documented attach command doesn't exist

**Observed.** The authoring skill says *"To connect to an existing Chrome
session: `sightmap browser register --addr localhost:PORT`"*. The agent ran
exactly that and got `unknown subcommand "register"` — which is what forced
the FM5 session-file archaeology.

**Root cause.** `go/cmd/sightmap/cmd_browser.go` has no `register`/`attach`
subcommand; `skills/sightmap-authoring/SKILL.md` documents one.

**Fix (PR-6).** Implement `browser register` (alias `attach`): probe
`/json/version`, write a proper session file, print the same `● running`
status as `start`. It's ~40 lines against existing helpers
(`cdpVersionAlive`, `WriteSessionInfo`).

**Evals.** `fm06-browser-register-missing`.

### FM7 — The skill teaches the broken shape

**Observed.** Before writing any YAML the agent read the installed
sightmap-authoring skill and followed it literally: *"Create
`.sightmap/views/PAGE.yaml` with `route: "/pattern/**"`"* and *"seed the
`route:` field of each `views/*.yaml`"*. In 495 lines the skill never once
shows a view file's top-level structure — `views:` does not appear in it. FM1
was the direct consequence.

**Root cause.** `skills/sightmap-authoring/SKILL.md` (canonical; `go/skills/`
is the generated copy). The full structure exists only in
`spec/v1/schema.md` / `docs/reference/schema.md`, which the CLI install
doesn't ship.

**Fix (PR-7).** Add a complete minimal view-file example (with `version: 1`
and the `views:` wrapper) to the skill's Phase 1a and its hard-rules section;
fix the `register` reference to match PR-6; add the `--port` vs `--cdp-port`
gotcha (FM12). Regenerate `go/skills/` via `go generate ./skills/...`.

**Evals.** `fm07-skill-doc-view-example` (docs-only case).

### FM8 — The tool warns about its own files

**Observed.** `warning: survey.yaml: unknown field "surveyed" (line 1)` —
survey.yaml is sightmap's own file, read by `discover`. Noise landed exactly
while the agent was isolating the real schema problem.

**Root cause.** `go/sightmap/loader.go:158` exempts only
`config.yaml`/`config.yml` from the corpus unknown-field walk.

**Fix (PR-9).** Exempt `survey.yaml` (and any future tooling files) — or move
tooling files out of the corpus glob.

**Evals.** `fm08-survey-yaml-corpus-warning`.

### FM9 — `browser start` fails blind

**Observed.** Four attempts failed with only *"chrome did not become ready:
timed out waiting for CDP at 127.0.0.1:7894"* — before **and after**
`browser install`. No binary path, no profile path, no Chrome stderr, no
next-step hint. Manual launches of both stable Chrome and Chrome for Testing
worked on the same machine, so the delta (sightmap's profile dir, extension
flags, and 10s deadline) had to be guessed.

**Root cause.** `go/cmd/sightmap/cmd_browser.go:139` `pollCDPReady` has a
fixed 10s deadline and `cmd_browser_start.go:220-222` kills Chrome and returns
a bare error on expiry; the `exec.Command` at `cmd_browser_start.go:213`
discards Chrome's stdout/stderr entirely. A cold first launch (fresh profile,
Gatekeeper checks on an older Intel Mac) can plausibly exceed 10s — and when
it does, the kill guarantees the next attempt is another cold start.

**Fix (PR-5).** Capture Chrome stderr into a bounded buffer; on timeout,
report the resolved binary, profile dir, full arg list, and the stderr tail;
make the deadline a `--start-timeout` flag (default 30s); on macOS print a
CfT-vs-stable-Chrome hint when the stable binary was selected.

**Evals.** `fm09-browser-start-no-diagnostics` (hermetic: a fake
`google-chrome` on PATH that logs to stderr and never opens CDP).

### FM10 — No scaffold at the entry point

**Observed.** The agent's *first* schema probe was `sightmap init --help` →
`unknown command "init"`. It then wrote the corpus from memory of the skill
text — incorrectly.

**Fix (PR-10).** Add `sightmap init`: scaffold `.sightmap/` with a commented
`components.yaml` and `views/example.yaml` that carry the correct shape
(`version: 1`, `views:` wrapper) so the first file an author sees is valid.
`discover` could later gain `--scaffold` to emit view stubs from crawled
routes, but plain `init` alone removes the blank-page failure.

**Evals.** `fm10-init-scaffold-missing`.

### FM11 — sel-probe's headline count is silently capped

**Observed.** `sel-probe -- 'tr[data-id]'` printed `matches: 10` against
`offline matcher: 142 matches`. The 10 is the `--max` *display* cap; the tool
already computes the true live total internally (`printOfflineCheck`'s
`liveN`, `go/cmd/sightmap/cmd_sel_probe.go:226-229`) but the headline prints
`len(results)` (`cmd_sel_probe.go:145`).

**Fix (PR-8).** Print `matches: 142 (showing 10)`.

**Evals.** `fm11-sel-probe-capped-count` (`requires: manual`).

### FM12 — `--port` means the server, not Chrome *(analysis-only)*

**Observed.** Mid-recovery, a `browser start` printed
`[serve-sightmap] listening on http://127.0.0.1:9222` with CDP tried at 7893 —
consistent with the agent passing `--port 9222` believing it set the CDP port.
The usage line `browser start [--headless] [--port N] [--url URL]` invites
exactly this reading.

**Fix.** Folded into PR-5/PR-7: flag help for `--port` says "sightmap HTTP
server port (not Chrome's CDP port — see --cdp-port)", and the top-level usage
mentions both.

### FM13 — Green checks reward deleting semantics *(analysis-only)*

**Observed.** Two structural quality regressions sailed through every check:
(a) to satisfy the `deep-nesting` lint, the agent **deleted**
`AdminViewLink`/`VariantsButton`/`DeleteButton` outright instead of
restructuring — coverage stayed "0 orphaned ✓" because T2 absorbs unnamed
interactives into any labeled ancestor; (b) `AdminTable: .table-wrap` matches
the *homepage* table wrapper too, mislabeling nodes across pages, invisible
because with zero views nothing is page-scoped. The only hard gate (T3 = 0)
can be satisfied by *removing* annotation.

**Fix (future work, not in the PR sequence).** Needs design: a T1-ratio floor
or a "named interactive" quality metric in `report`, and a multi-page
exclusive-component heuristic in `lint`. Eval cases should be added when the
metric is designed; the transcript moments are tagged for it.

## Recommended PR sequence

Small and surgical, ordered so each lands independently; agent-time-saved ÷
diff-size descending. Each PR flips its eval cases from `known` to `pass`
(flip `known_failing` in the same PR — the runner errors on unclaimed fixes).

| # | PR | FMs | Touches | Size | Evals flipped |
|---|---|-----|---------|------|----------------|
| PR-1 | `--help` exits 0; drop "flag: help requested" | FM4 | `cmd/sightmap/main.go` | ~5 lines | fm04 |
| PR-2 | View-file trap diagnostics: view-field-at-root message with `views:` example; viewless-file warning; viewless-corpus warning; missing `version:` warning | FM1, FM3 | `sightmap/unknownfields.go`, `sightmap/loader.go`, `sightmap/validate.go` | ~80 lines + tests | fm01×3, fm03 |
| PR-3 | `report`/`capture` errors teach the schema and agree with each other | FM2 | `cmd/sightmap/cmd_report.go`, `cmd_capture.go` | ~30 lines | fm02 |
| PR-4 | Session-file honesty: reject port-less JSON with expected shape; gate the "same port" hint on `ServerPort > 0`; document `.session` | FM5 | `browser/launcher.go`, `cmd/sightmap/cmd_browser.go`, `docs/cli/browser.mdx` | ~40 lines | fm05 |
| PR-5 | `browser start` diagnosability: capture Chrome stderr, report binary/profile/args on timeout, `--start-timeout` (default 30s), clarify `--port` vs `--cdp-port` | FM9, FM12 | `cmd/sightmap/cmd_browser_start.go`, `cmd_browser.go` | ~80 lines | fm09 |
| PR-6 | `browser register` (alias `attach`) — the documented attach path | FM6 | `cmd/sightmap/cmd_browser.go` | ~40 lines | fm06 |
| PR-7 | Skill sync: full `views:` example, register/attach reference, port-flag gotcha; regenerate `go/skills/` | FM7, FM12 | `skills/sightmap-authoring/SKILL.md` (+generated) | docs | fm07 |
| PR-8 | sel-probe headline shows true live count: `matches: M (showing N)` | FM11 | `cmd/sightmap/cmd_sel_probe.go` | ~10 lines | fm11 (manual) |
| PR-9 | Exempt tooling files (`survey.yaml`) from corpus unknown-field warnings | FM8 | `sightmap/loader.go` | ~5 lines | fm08 |
| PR-10 | `sightmap init` scaffold with schema-correct commented examples | FM10 | new `cmd/sightmap/cmd_init.go`, `main.go` | ~80 lines | fm10 |

Rationale for the order: PR-1 is trivial and de-noises everything after it.
PR-2 + PR-3 kill the failure that consumed most of the session and would have
each rescued it *independently* — the trap (FM1) plus either honest validate
output or one accurate error message ends the guessing loop. PR-4/5/6 are the
browser-session cluster that consumed the rest. PR-7 lands after PR-6 so the
skill documents a command that exists. PR-8/9/10 are polish. FM13 is future
work pending metric design.

CI wiring: add `evals/run.sh` as a job next to `go test ./...` — the suite is
green today (`pass=3 known=12 skip=1`) and stays green until a regression or
an unclaimed fix.
