# Atlas AXIS scorecards — specification

Status: **Draft**, for maintainer review. Nothing here is implemented.

Defines how the Sightmap Atlas measures, records, and publishes an **AXIS score**
for every entry: what is run, how the before/after pair is constructed, what
counts as a publishable result, and the exact shape of the data that reaches
`index.json`, sightmap.org, and `llms.txt`.

This document is the contract. [`PLAN.md`](PLAN.md) is the build order.

Companion contracts that must not drift: atlas [`docs/SPEC.md`](https://github.com/sightmap/atlas/blob/main/docs/SPEC.md)
(entry format, `index.json`) and [`docs/POLICY.md`](https://github.com/sightmap/atlas/blob/main/docs/POLICY.md)
(what may be captured and how). Where this document extends `index.json`, that
extension belongs in atlas `docs/SPEC.md` when it is accepted — this file is the
proposal, not the second source of truth.

---

## 1. What AXIS is

[AXIS](https://axis.run) (Agent Experience Index Score) is Netlify's open-source
harness for measuring how well a service works *for an AI agent*. Lighthouse, but
the user is an agent. MIT, `npm i -g @netlify/axis`, developed in the open at
[netlify/axis](https://github.com/netlify/axis) with Auth0 and Resend as founding
contributors.

The parts that matter for us:

**Execution model.** `axis run` reads `axis.config.{json,js,ts}`, discovers
scenarios, and runs every scenario × agent pair as an independent job in a fresh
temp workspace with an isolated `HOME` (so `.claude/`, `.codex/` never appear in
the directory the agent scans). Concurrency defaults to 15. Each job is
setup → spawn → capture transcript → score → teardown → save. Default per-scenario
limit is 15 minutes; time and token limits are configurable per run and per
scenario.

**A scenario** is a JSON/JS/TS file: `name`, `prompt`, `judge` (a string or an
array of `{check, weight}` rubric items), plus optional `setup` / `teardown`
lifecycle actions (`run_script`, `copy`), `skills`, `mcp_servers`, `limits`,
`artifacts`, and `variants`. The filename becomes the scenario key.

**Variants** are the feature this whole design rests on. A scenario with
`variants: [...]` becomes a template; each variant is an independent job that
inherits the parent's prompt and rubric and overrides only what it names
(`skills`, `mcp_servers`, `setup`, `prompt`, …). Keys are suffixed
`scenario@variant`. A variant with no overrides is the documented control
pattern. **One prompt, one rubric, two tool configurations, scored identically.**

**Scoring** produces a 0–100 composite from four dimensions:

| Dimension | Weight | What it measures | Signals |
|---|---|---|---|
| Goal Achievement | 0.40 | Your rubric, graded 0–10 per check by an LLM judge reading the full transcript | judge only |
| Environment | 0.20 | Did local tools (shell, fs, git, package managers, build) succeed and respond quickly | Success 0.7, Speed 0.3 |
| Service | 0.20 | Did external APIs, MCP tools, and network calls succeed and respond quickly | Success 0.7, Speed 0.3 |
| Agent | 0.20 | Decision quality across *every* interaction | Necessity 0.4, Weight 0.2, Relevance 0.2, Success 0.1, Speed 0.1 |

Weights are configurable and must sum to 1.0. Raw signals map to 0–100 through a
log-normal S-curve where 50 is median performance, so late gains cost more than
early ones. Speed is heuristic (measured from transcript timestamps, category-specific
thresholds); the other signals are judge-evaluated. Bands: ≥90 green, 80–89
yellow, 70–79 orange, <70 red; below 75 the CLI emits insights naming the weakest
signal.

The deliberate split — Environment/Service score *execution* quality, Agent scores
*decision* quality — is why this is a usable instrument for us. "The agent read
nine pages it did not need" lands in Agent (Necessity), not in Environment.

**Judging.** By default the agent under test grades its own transcript. Setting
`judging.agents` to a precedence-ordered list makes AXIS pick the first candidate
whose adapter differs from the run's own agent.

**Reports and baselines.** Every run writes `.axis/reports/<id>/` — `report.json`
manifest, `report.html`, and per-job JSON with the full transcript, per-check judge
evaluations, and per-interaction signal scores. `axis baseline set` snapshots
scores; `axis run --compare-baseline` exits 1 on any regression beyond a 1-point
noise tolerance. Baselines are meant to be committed; reports are not.

Relevant roadmap items, not yet shipped: an embeddable AXIS badge, a judge model
configurable independently of the agent under test, and CI score thresholds.

---

## 2. What we are measuring, and why this shape

The atlas asserts something testable: **an agent working against a mapped site
does better than an agent working against the same site unmapped.** Today that
claim is unmeasured. AXIS turns it into a number per entry.

### 2.1 Before/after is an A/B, not a then/now

The obvious reading of "score before and after the sightmap is built" is two runs
separated in time. We reject that:

- Atlas entries map **live third-party production sites**. Amazon in March is not
  Amazon in June. A then/now delta is contaminated by every A/B test, layout
  change, and geo variation that landed in between.
- The unmapped run has to happen before the map exists, so it can never be
  re-run to check it, and it can never be re-run against the same site state.

Instead, both sides run **in the same AXIS invocation, minutes apart, against the
same site state**, as two variants of one scenario. Site drift is common-mode and
mostly cancels. The pair is re-runnable forever.

### 2.2 The variants

Every atlas scenario defines exactly two required variants:

| Variant | Workspace | What it isolates |
|---|---|---|
| `@cold` | sightmap CLI and browser tooling present; **no `.sightmap/` corpus, no sightmap skills** | the value of *the map* |
| `@map` | same tooling, plus the entry's corpus installed and the sightmap skills available | — |

**`@map` − `@cold` is the published lift.** Both sides get identical browser
capability, so the number is attributable to the corpus and its skills, not to
"we gave one agent a browser."

A third variant is optional and must be labelled distinctly:

| Variant | Workspace | What it isolates |
|---|---|---|
| `@raw` | no sightmap tooling at all — the agent browses with whatever it has (WebFetch, curl, its own Playwright) | the whole stack vs. the status quo |

`@raw` produces the larger, more quotable number and supports the weaker claim.
It may be measured and shown, never as the headline, and never mixed into `lift`.

### 2.3 What we expect to move, and what we do not

Predicting this up front is how we tell a real result from a rigged one.

- **Goal Achievement** should move most. A map that names `ListingDetail`'s
  price component turns "hunt for the price" into "read a named component."
- **Agent** should move next. Necessity carries 0.4 and a map's whole purpose is
  eliminating exploratory flailing — fewer snapshots, fewer speculative clicks.
- **Environment** should barely move, and may move *down*: the `@map` variant
  runs more CLI commands. That is honest and must be reported, not hidden.
- **Service** should barely move at all today. The sightmap surface is a CLI, so
  its calls classify as Environment. Service becomes the dimension to watch only
  if sightmap ever ships an MCP server — worth knowing before someone reads a
  flat Service delta as a failure.

A result where Goal Achievement is flat and only Environment moves is not a win;
it is an artifact. §6 makes that non-publishable.

---

## 3. Scenario contract

### 3.1 Archetypes

A score of 79 on airbnb and 61 on nike compares nothing unless both were asked
comparable questions. Every scenario therefore instantiates one **archetype** from
a closed set, and cross-entry comparison is only valid within a shared archetype
mix.

| Archetype | The task | Why a map should help |
|---|---|---|
| `navigate` | Reach a specific named view from the site root and prove you are on it | routes and view names are in the corpus |
| `extract` | Report a named property from a named component on a specific view | the component and its properties are named; no DOM hunting |
| `traverse` | Cross two or more views in a read-only flow (search → result → detail) and report a fact from the last one | the map records the path and the per-view components |
| `recover` | Land on a broken or unmatched URL and correctly report what the site did | `NotFound` is usually a mapped view; unmapped 200s are a documented quirk |
| `request` | Name the network request a given interaction fires, and one of its properties | `requests[]` is corpus data an unmapped agent must infer |

An entry ships **3–5 scenarios**, at least one `navigate` and one `extract`.
`traverse` is recommended for any entry with more than three views. New archetypes
require a spec change here, not a one-off scenario.

### 3.2 Hard rules for prompts and rubrics

These exist because we are grading our own product.

1. **The prompt is identical across variants.** It names the site and the goal in
   the site's own vocabulary. It never says "sightmap," never names a corpus file,
   and never hints that a map exists. A variant that overrides `prompt` is invalid
   under this spec.
2. **The rubric is identical across variants and outcome-only.** Every check is
   observable in the transcript or in a captured artifact.
   `"Agent reported the listed price of the first search result"` — yes.
   `"Agent used the sightmap corpus"` — never. Rewarding tool use rigs Goal
   Achievement and is exactly the weak check AXIS's own guidance warns against.
3. **No answers in the rubric.** If the expected value is in the check text, the
   judge grades the prompt, not the run.
4. **Read-only.** No purchases, no account creation, no form submission that
   mutates third-party state, no auth bypass, no CAPTCHA circumvention, no
   hammering. POLICY.md governs scenarios exactly as it governs corpora.
5. **Answers that drift are asked as shapes, not values.** "Report the price
   shown" (judge checks a plausible currency value was read off the page), not
   "report that the price is $129."

### 3.3 Layout

```
entries/<slug>/
  axis/
    scenarios/
      01-navigate-listing.json
      02-extract-price.json
      03-traverse-search.json
    result.json            # GENERATED — latest published measurement
    history/
      2026-08-20.json      # GENERATED — one record per measurement
```

`axis/` is optional. An entry without it is a valid entry; it simply carries no
score.

### 3.4 A scenario, concretely

```json
{
  "name": "Read the nightly price on a listing page",
  "archetype": "extract",
  "prompt": "On airbnb.com, open any listing in Paris and report the nightly price shown on the listing page, plus the name of the section it appears in.",
  "judge": [
    { "check": "Agent reached a listing detail page (URL matches /rooms/<id>)", "weight": 0.3 },
    { "check": "Agent reported a nightly price as a currency amount read from that page", "weight": 0.5 },
    { "check": "Agent named the page region the price appears in", "weight": 0.2 }
  ],
  "artifacts": ["answer.md", "*.png"],
  "limits": { "time_minutes": 12, "tokens": 250000 },
  "variants": [
    { "name": "cold" },
    {
      "name": "map",
      "setup": [
        { "action": "run_script", "command": "$AXIS_CONFIG_DIR/axis/bin/bootstrap-chrome.sh" },
        { "action": "run_script", "command": "sightmap atlas add \"$ATLAS_SLUG\"" },
        { "action": "run_script", "command": "sightmap skills install --target .claude/skills" }
      ]
    }
  ]
}
```

`archetype` is not an AXIS field; AXIS ignores unknown keys and our own tooling
reads it. If that ever stops being true, it moves into the scenario filename
convention instead.

The parent scenario carries the shared `setup` (Chrome bootstrap only). `cold`
inherits it unchanged — that is the whole control. `map` replaces it with the
same bootstrap plus corpus and skills, because AXIS variant `setup` **replaces**
rather than merges.

### 3.5 Harness configuration

`axis.config.ts` lives at the atlas repo root. Requirements:

- **Pin the harness.** `@netlify/axis` is a devDependency at an exact version.
  A harness upgrade can move scores without anything about the map changing, so
  the version is part of every published record (§4) and a bump invalidates
  baselines.
- **Pin agent and model.** One canonical pair produces published scores. Others
  may be run for research and are recorded but not published.
- **Third-party judge.** `judging.agents` must resolve to an adapter different
  from the agent under test. Non-negotiable: self-graded scores about our own
  product are not evidence.
- **Default dimension weights.** Do not tune `scoring_weights`. A custom weighting
  that happens to favour our result is indefensible, and comparability with any
  other AXIS score dies with it.
- **Low concurrency.** These jobs drive a real browser against live third-party
  sites. Cap at 2–3, never the default 15.
- **Chrome once, not per job.** `sightmap browser install` writes ~184 MB to
  `~/.sightmap/browsers/`, and AXIS gives every job its own `HOME`. `beforeAll`
  installs into a shared path; each job's setup symlinks
  `$HOME/.sightmap/browsers` at it. Both variants get the identical binary, so
  this cannot bias the pair.
- **Limits.** Per-scenario time and token limits are mandatory. An unbounded
  browser-driving agent on a live site is a cost incident.

---

## 4. The result record

### 4.1 `axis/result.json` (generated, never hand-authored)

Same rule as `stats`: computed by tooling from `report.json`, never typed by a
contributor.

```json
{
  "schema_version": 1,
  "slug": "airbnb",
  "measured": "2026-08-20",
  "entry_commit": "9f3c…",
  "harness": "@netlify/axis@0.9.3",
  "agent": "claude-code",
  "model": "claude-opus-5",
  "judge": "codex",
  "tier": "live",
  "region": "us-east",
  "archetypes": ["navigate", "extract", "traverse"],
  "scenarios": 3,
  "runs_per_variant": 5,
  "cold":  { "result": 61, "goal_achievement": 58, "environment": 74, "service": 66, "agent": 55, "spread": 7 },
  "map":   { "result": 79, "goal_achievement": 84, "environment": 71, "service": 68, "agent": 76, "spread": 4 },
  "lift":  { "result": 18, "goal_achievement": 26, "environment": -3, "service": 2, "agent": 21 },
  "confidence": "clear",
  "report_url": "https://sightmap.org/atlas/axis/airbnb/2026-08-20.html",
  "per_scenario": [
    { "key": "02-extract-price", "archetype": "extract", "cold": 54, "map": 82, "lift": 28 }
  ]
}
```

Field notes:

- **`cold` / `map`** are the **median** across `runs_per_variant` repetitions per
  scenario, then the mean across scenarios. Median first because a single
  CAPTCHA or timeout should not define an entry.
- **`spread`** is the interquartile range of the composite across repetitions. It
  is the honesty field: a lift of 6 with spreads of 20 is noise, and §6 refuses
  to publish it.
- **`entry_commit`** is the entry's commit at measurement time. §5 uses it to
  detect a score that has gone stale against edited content.
- **`tier`** ∈ `live` | `self-hosted` | `replay` (see §7).
- **`confidence`** ∈ `clear` | `weak` | `none`, derived per §6. Never authored.
- **`region`** is recorded because a geo-varying site scores differently from
  different egress.

`history/<date>.json` is the same shape, appended each measurement, never
rewritten. `result.json` is a copy of the most recent publishable record.

### 4.2 The `axis` block in `index.json`

`gen-index` folds a compact projection of `result.json` into each entry. Consumers
(the gallery, `llms.txt`, `sightmap atlas find`) read only this.

```json
"axis": {
  "measured": "2026-08-20",
  "entry_commit": "9f3c…",
  "current": true,
  "tier": "live",
  "confidence": "clear",
  "agent": "claude-code",
  "model": "claude-opus-5",
  "harness": "@netlify/axis@0.9.3",
  "cold": 61,
  "map": 79,
  "lift": 18,
  "lift_goal": 26,
  "lift_agent": 21,
  "report_url": "https://sightmap.org/atlas/axis/airbnb/2026-08-20.html"
}
```

- `axis` is **optional**. Every consumer must render an entry that has none. This
  is not negotiable — most entries will have no score for most of the atlas's
  life, and the gallery must not degrade into a wall of "unscored."
- `current` is `axis.entry_commit === entry.commit`. False means the entry
  changed after it was measured.
- `cold`, `map`, and `lift` always travel together. Publishing `map` alone would
  invite reading it as an absolute grade of the *site*, which it is not.

### 4.3 What `axis` is not

Not a rating of the mapped site. Not a rating of the agent. It is a measurement
of **one map's usefulness to one agent on one day**, and every field that
qualifies it — date, agent, model, harness, tier, confidence — travels with the
number everywhere it is shown.

---

## 5. Publication and display

**Card (`/atlas`).** One badge: `AXIS +18`, coloured by confidence, not by score
band. Unscored entries show nothing — no placeholder, no "not yet scored" chip.
Sorting by lift is allowed; it must place unscored entries last rather than at
zero.

**Detail page (`/atlas/:slug`).** A scorecard section: the pair as two bars, the
four per-dimension deltas (including negative ones), agent/model/harness/date/tier,
a link to the HTML report, and one sentence of plain-language methodology linking
to this spec. Negative dimension deltas render exactly as measured.

**Stale scores.** When `current` is false the scorecard renders with a "measured
against an earlier version of this map" note and the lift is **excluded from any
sort or aggregate**. A stale score is shown for provenance, never counted.

**Machine twins.** `/atlas/index.json` carries the block verbatim. `/atlas/<slug>.md`
gains an AXIS line in its front matter. The `llms.txt` atlas section gains one
clause per scored entry — `AXIS +18 (claude-code, 2026-08-20)` — because an agent
choosing between exploring by hand and installing a map is precisely the audience
this number is for.

**Aggregates.** Any atlas-wide figure ("+21 median lift across 12 entries") is
computed only over `current: true`, `confidence: "clear"` records sharing one
agent and model, and must state n, the agent, and the archetype mix. A blended
number across mixed agents is not a number.

---

## 6. Publishability

A measurement is published only if **all** hold:

1. `runs_per_variant ≥ 3` (target 5).
2. Both variants completed on at least 3 scenarios; a job that timed out or died
   is excluded from the median and counted in a `failed` tally, and if more than
   a third of jobs failed the whole measurement is void.
3. The judge adapter differs from the agent under test.
4. Dimension weights are AXIS defaults.
5. `|lift.result| > max(cold.spread, map.spread)` → `confidence: "clear"`.
   Otherwise `weak`. Only `clear` is published as a headline; `weak` is written to
   `history/` and shown on the detail page as "within run-to-run noise."
6. **`lift.goal_achievement > 0` whenever `lift.result > 0`.** A composite lift
   carried entirely by Environment or Service — the agent's tools got faster
   without the task going better — is `confidence: "none"` and is never published.
   This is the specific way this measurement could flatter us, so it is blocked in
   the contract rather than left to reviewer judgement.
7. Scenarios, `axis.config.ts`, `result.json`, and the HTML report are all
   committed or published. An unauditable score is not published.

A negative lift is published like any other result. A map that does not help is a
finding, and suppressing it would make every positive number worthless. In
practice a negative lift is usually a **stale map** — selectors that no longer
match cost the agent time — which makes AXIS a second, sharper freshness signal
feeding the same staleness automation as P5.1.

---

## 7. Tiers, and the live-site problem

Scoring against live third-party production sites is the honest thing to measure
and the hardest thing to measure reliably: bot defenses, geo and A/B variation,
CAPTCHAs, and layout churn all move the score without anything about the map
changing. POLICY.md's "no rate hammering, no auth bypass, no CAPTCHA
circumvention" applies in full.

| Tier | What it is | Status |
|---|---|---|
| `live` | Read-only tasks against the public site, low concurrency, scheduled not per-merge, single region | supported at launch |
| `self-hosted` | The entry maps OSS we run ourselves; fully deterministic | supported at launch, preferred for anything used in methodology writing |
| `replay` | Scored against a frozen capture of the site | **not** in scope; reserved so the field does not have to change later |

Two consequences: seed and demonstrate on `self-hosted` and on our own properties
first, where a bad number means a bad map rather than a bad afternoon on someone
else's CDN; and never gate an atlas contribution on a `live` score — see §8.

---

## 8. Non-goals

- **No admission gate.** A minimum lift is not required to merge an entry. Cost
  and `live` variance both make it unfair at launch, and it would push
  contributors toward maps that score rather than maps that are true. Revisit once
  there are enough measurements to know what the distribution even looks like.
- **No new CLI verb.** No `sightmap axis`. AXIS is the harness; reimplementing any
  part of it inside our CLI buys nothing and forks the numbers. The `@map` setup
  is already expressible with shipped verbs: `sightmap atlas add <slug>` and
  `sightmap skills install --target`.
- **No spec change.** Nothing here touches `spec/v1/`, the YAML schema, or the
  meaning of any sightmap field. No SEP is required. The `index.json` extension in
  §4.2 is an atlas contract and lands in atlas `docs/SPEC.md`.
- **Not a site rating.** See §4.3. Any UI or copy that reads as grading the mapped
  site is a bug against this spec.
