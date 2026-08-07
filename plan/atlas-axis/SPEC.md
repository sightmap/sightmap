# Atlas AXIS scorecards — specification

Status: **Draft**, for maintainer review. Nothing here is implemented.

Defines how the Sightmap Atlas measures, records, and publishes an **AXIS score**
for every entry: what is run, how the variant ladder is constructed, what counts
as a publishable result, and the exact shape of the data that reaches
`index.json`, sightmap.org, and `llms.txt`.

This document is the contract. [`PLAN.md`](PLAN.md) is the build order.

Companion contracts that must not drift: atlas [`docs/SPEC.md`](https://github.com/sightmap/atlas/blob/main/docs/SPEC.md)
(entry format, `index.json`) and [`docs/POLICY.md`](https://github.com/sightmap/atlas/blob/main/docs/POLICY.md)
(what may be captured and how). Where this document extends `index.json`, that
extension belongs in atlas `docs/SPEC.md` when it is accepted — this file is the
proposal, not the second source of truth.

Shaped by a thread with Sean Roberts (Netlify), who maintains AXIS. His guidance
is reflected throughout; §2.4 records where he told us we are off the beaten path.

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
`artifacts`, and `variants`. The filename becomes the scenario key. TS scenarios
default-export a `ScenarioInput`, which lets shared helpers generate the
repetitive parts — the pattern Sean pointed us at and the one §3.4 adopts.

**Variants** are the feature this whole design rests on. A scenario with
`variants: [...]` becomes a template; each variant is an independent job that
inherits the parent's prompt and rubric and overrides only what it names
(`skills`, `mcp_servers`, `setup`, `prompt`, `judge`, …). Keys are suffixed
`scenario@variant`. **One prompt, one rubric, several tool configurations, scored
identically.** Sean's framing: *"1 variant is no additional context, another with
some other source like skills, and one with sightmap. Then when you run that
scenario, it runs all of the variants and you can see how a given scenario is
impacted by what you give it."*

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

**The canonical reference implementation** is
[netlify/context-and-tools](https://github.com/netlify/context-and-tools/tree/main/axis-scenarios) —
Netlify's own scenarios for validating their skills. It is worth reading before
writing a line of ours: ~16 capability folders of TS scenarios, a
`helpers/variants.ts` exporting `withSkillVariants()` (the `no-context` /
`with-skill` pair), and `helpers/setup.ts` exporting `copyFixture()`. Their rubric
style is instructive — many unweighted, very specific checks, including negative
ones (*"Does NOT direct the user to the dashboard UI"*) and explicit
vacuous-pass allowances.

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
- The unmapped run would have to happen before the map exists, so it could never
  be re-run to check it, and never against the same site state.

The map already exists by the time we measure, so nothing forces the runs apart.
All variants of a scenario run **in the same invocation against the same site
state** — concurrently where the concurrency cap allows. The requirement is that
a scenario's variants are scheduled as one contiguous group and are never split
across a schedule boundary or a partial re-run; §7's politeness cap on `live`
entries governs how many run at once, not whether they stay together.

### 2.2 The variant ladder

Following Sean's structure, every atlas scenario defines three variants. Names
match the `netlify/context-and-tools` convention where they mean the same thing.

| Variant | Workspace | Answers |
|---|---|---|
| `no-context` | nothing added — the agent browses with whatever it already has (WebFetch, curl, its own automation) | what does an agent do here today? |
| `with-skills` | the sightmap CLI and the `sightmap-browser` / `sightmap-authoring` skills, **no corpus** | how much of it is just having browser tooling? |
| `with-sightmap` | the same skills **plus the entry's `.sightmap/` corpus** | how much does the map itself add? |

Two deltas fall out, and they answer different questions:

- **`lift` = `with-sightmap` − `with-skills`.** The headline. Both sides have
  identical browser capability, so the number is attributable to the corpus
  alone. This is the atlas's actual claim.
- **`lift_total` = `with-sightmap` − `no-context`.** The whole stack versus the
  status quo. A larger and more quotable number supporting a weaker claim. It is
  recorded and may be shown, clearly labelled, but it is never the headline.

The middle rung is the one that earns trust. Without it, any critic can say we
measured "a browser versus no browser" and be right.

### 2.3 What we expect to move, and what we do not

Predicting this up front is how we tell a real result from a rigged one.

- **Goal Achievement** should move most between `with-skills` and
  `with-sightmap`. A map that names `ListingDetail`'s price component turns "hunt
  for the price" into "read a named component."
- **Agent** should move next. Necessity carries 0.4 and a map's whole purpose is
  eliminating exploratory flailing — fewer snapshots, fewer speculative clicks.
- **Environment** should barely move, and may move *down*: `with-sightmap` runs
  more CLI commands. That is honest and gets reported, not hidden.
- **Service** should barely move at all — for the structural reason in §2.4.

A result where Goal Achievement is flat and only Environment moves is not a win;
it is an artifact. §6 makes that non-publishable.

### 2.4 Where this is off the beaten path

Sean was direct about it. AXIS today is aimed at *"do I have what I need set up
such that the agent could do well"* — an audit of a service's agent-facing
surface — more than at scoring an agent's competence at operating a UI. He thinks
it can be extended to computer use and would like to see it, but we should expect
to be first, and the plan should not pretend otherwise.

The concrete mechanical consequence, which P6.0 must confirm or refute:

**The site under test is largely invisible to the Service dimension.** AXIS
classifies interactions by tool. Our agent drives the browser through
`sightmap browser …` shell commands, so those interactions classify as
**Environment** — local tooling. The thing we actually care about, the website,
contributes almost nothing to Service. So the 0.20 of the composite meant for
"how well did the external service behave" is measuring the wrong external
service, and the site's own behaviour shows up only indirectly, through Goal
Achievement and Agent.

Three responses, in the order we should try them:

1. **Lead with the dimensions that do work.** Goal Achievement (0.40) and Agent
   (0.20) are both meaningful here and are the two we predicted would move. §5
   surfaces the per-dimension deltas rather than the composite alone precisely
   because the composite carries dead weight for this use case.
2. **An MCP surface for sightmap** would reclassify browser interactions as
   Service and make that 0.20 mean something. It is a real option, out of scope
   here, and worth noting as a benefit if that work is ever considered on its own
   merits.
3. **Talk to Sean about a browser/computer-use interaction category.** He is
   receptive, no one has benchmarked agent experience for general browser use,
   and the finding in (1) is exactly the kind of upstream signal that would inform
   it. This is a collaboration opportunity, not a blocker.

### 2.5 The other track, named so it does not get conflated

There are two distinct things AXIS could score for us:

- **Track A — the sightmap spec and tooling themselves.** Can an agent, given our
  skills and CLI, correctly author and maintain a corpus? This is the conventional
  AXIS use case and maps almost one-to-one onto `netlify/context-and-tools`: no
  browser, no live sites, no novel dimension behaviour, and a genuinely useful
  quality signal for the CLI and the skills.
- **Track B — the atlas scorecard.** This document. Novel, browser-driven,
  live-site, per-entry.

They share a harness and almost nothing else. Track A is out of scope here, but
it is cheap, low-risk, and would build real AXIS experience before Track B needs
it — see the optional precursor in PLAN.md.

**A third shape, for later.** Sean's description of AXIS's design centre — an
*audit* of whether the agent has what it needs to do well — suggests the mature
form of this phase is two-layered, like web performance tooling: a cheap,
deterministic **audit** of each entry on every merge (does the corpus cover the
site's main routes, are components propertied, is memory present, are selectors
fresh — Lighthouse's lab data), with the run-based lift as the expensive,
scheduled field measurement (the RUM). The audit layer overlaps `sightmap lint`
and is not designed here; it is named so that if the lift proves too expensive to
run often, the per-merge quality signal already has a shape waiting.

### 2.6 Known threats to validity

Named here so the spike measures them instead of discovering them later.

**Judge-legibility bias.** Goal Achievement is an LLM judge reading the
transcript. A `with-sightmap` transcript is clean annotated component trees; a
`no-context` transcript may be raw fetched HTML. The judge can verify checks more
easily in the legible transcript, and unverifiable claims score lower — so part
of any measured lift could be transcript readability rather than task success.
Two mitigations are mandatory: every scenario requires a written answer artifact
(`answer.md`, captured via `artifacts`) so checks are groundable in a file rather
than in prose scattered through the transcript; and P6.0 human-grades every job
and reports judge–human agreement **per rung**. If agreement differs materially
by rung, the instrument is biased and nothing is published until that is
understood.

**Composite dilution.** With Service dead (§2.4) and Environment intentionally
flat, composite lift ≈ 0.4·ΔGoal + 0.2·ΔAgent. The §4.1 example — +26 Goal, +21
Agent — publishes as roughly +15. The composite systematically understates the
dimensions this measurement is about. The gallery therefore always shows
per-dimension deltas (§5), and whether the headline badge is the composite lift
or the Goal Achievement lift is an open decision (PLAN, open questions), made
after the spike and **with Netlify** — publishing a re-weighted or re-labelled
number under the AXIS name without their agreement is off the table.

**Empty-dimension behaviour is unknown.** The AXIS docs do not say what a
dimension with zero interactions scores. If empty Service defaults high, it
dilutes uniformly (tolerable); if it swings on one or two stray calls, it injects
noise into a fifth of the composite. P6.0 answers this empirically before any
schema is built.

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
   `"Agent used the sightmap corpus"` — never.
3. **No answers in the rubric.** If the expected value is in the check text, the
   judge grades the prompt, not the run.
4. **Read-only.** No purchases, no account creation, no form submission that
   mutates third-party state, no auth bypass, no CAPTCHA circumvention, no
   hammering. POLICY.md governs scenarios exactly as it governs corpora.
5. **Answers that drift are asked as shapes, not values.** "Report the price
   shown" (judge checks a plausible currency amount was read off the page), not
   "report that the price is $129."
6. **Checks are rung-neutral and grounded in an artifact.** Every scenario
   instructs the agent to write its findings to `answer.md` and captures it via
   `artifacts`, and every check is phrased so it is satisfiable in principle from
   any rung's transcript. A check that assumes a browser exists ("the final URL
   matches…") silently fails the `no-context` rung for the wrong reason; phrase
   it as "identifies a specific listing page (its `/rooms/<id>` URL) as the
   source of the answer" instead. Where prior knowledge could fake success
   (install commands, well-known facts), add a check that the transcript shows
   the answer was read from the site rather than recalled.

**On rule 2 and `withSkillVariantsStrict`.** AXIS supports a per-variant `judge`
override, and Netlify uses it deliberately: their `with-skill` runs are held to a
stricter rubric that demands the *recommended* path, because for them "used the
product correctly" is the outcome. Our situation differs — the task is to
accomplish something on a website, and the map is an aid, not the goal — so a
stricter `with-sightmap` rubric would be grading the tool we are trying to
evaluate. We decline it for anything published. It stays available for unpublished
research runs (*"did the agent take the path the map suggests?"*), which is a real
question, just not this measurement.

### 3.3 Layout

```
entries/<slug>/
  axis/
    scenarios/
      01-navigate-listing.ts
      02-extract-price.ts
      03-traverse-search.ts
    result.json            # GENERATED — latest published measurement
    history/
      2026-08-20.json      # GENERATED — one record per measurement
```

`axis/` is optional. An entry without it is a valid entry; it simply carries no
score.

### 3.4 Scenarios are TypeScript, and the ladder lives in one helper

Following the `netlify/context-and-tools` pattern: the variant ladder, the Chrome
bootstrap, and the corpus install are repetitive and safety-critical, so they live
in one shared helper rather than being copy-pasted into every entry. An entry's
scenario file then carries only what is actually specific to it.

```ts
// axis/helpers/variants.ts
import type { ScenarioInput } from "@netlify/axis";

type Variant = NonNullable<ScenarioInput["variants"]>[number];

const SIGHTMAP_SKILLS = ["./skills/sightmap-browser", "./skills/sightmap-authoring"];

// Chrome for Testing is ~184 MB and AXIS gives every job its own HOME, so the
// binary is installed once in beforeAll and each job symlinks at it. Both rungs
// that browse get the identical binary, so this cannot bias the ladder.
const chrome = { action: "run_script", command: "$AXIS_CONFIG_DIR/axis/bin/link-chrome.sh" } as const;

// The three-rung ladder every atlas scenario runs. `no-context` gets nothing;
// `with-skills` isolates the tooling; `with-sightmap` adds the map. Variant
// `setup` REPLACES the parent's rather than merging, so each rung restates what
// it needs.
export function sightmapVariants(slug: string): Variant[] {
  return [
    { name: "no-context", setup: [] },
    { name: "with-skills", skills: SIGHTMAP_SKILLS, setup: [chrome] },
    {
      name: "with-sightmap",
      skills: SIGHTMAP_SKILLS,
      setup: [chrome, { action: "run_script", command: `sightmap atlas add ${slug}` }],
    },
  ];
}
```

```ts
// entries/airbnb/axis/scenarios/02-extract-price.ts
import type { ScenarioInput } from "@netlify/axis";
import { sightmapVariants } from "../../../../axis/helpers/variants";

export default {
  name: "Airbnb: read the nightly price on a listing page",
  archetype: "extract",
  prompt:
    "On airbnb.com, open any listing in Paris and report the nightly price shown on the listing page, plus the name of the page section it appears in. Write your findings to answer.md, including the URL of the listing you used.",
  judge: [
    { check: "answer.md identifies a specific listing detail page (its /rooms/<id> URL) as the source of the answer" },
    { check: "Reports a nightly price as a currency amount actually read off that page (visible in the transcript), not inferred from search results" },
    { check: "Names the page region the price appears in" },
    { check: "Does NOT attempt to book, sign in, or submit any form" },
  ],
  artifacts: ["answer.md", "*.png"],
  limits: { time_minutes: 12, tokens: 250000 },
  variants: sightmapVariants("airbnb"),
} satisfies ScenarioInput;
```

`archetype` is not an AXIS field; AXIS ignores unknown keys and our own tooling
reads it. If that ever stops being true it moves into the filename convention.

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
  product are not evidence. (Netlify's own config judges with `codex`.)
- **Default dimension weights.** Do not tune `scoring_weights`. A custom weighting
  that happens to favour our result is indefensible, and comparability with any
  other AXIS score dies with it. §2.4's finding is reported, not weighted away.
- **Low concurrency.** These jobs drive a real browser against live third-party
  sites. Cap at 2–3, never the default 15 — while keeping a scenario's variants
  contiguous per §2.1.
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
  "runs": { "no_context": 2, "with_skills": 5, "with_sightmap": 5 },
  "failed": { "no_context": 1, "with_skills": 0, "with_sightmap": 0 },
  "rungs": {
    "no_context":    { "result": 48, "goal_achievement": 41, "environment": 70, "service": 62, "agent": 44, "spread": 9 },
    "with_skills":   { "result": 61, "goal_achievement": 58, "environment": 74, "service": 66, "agent": 55, "spread": 7 },
    "with_sightmap": { "result": 79, "goal_achievement": 84, "environment": 71, "service": 68, "agent": 76, "spread": 4 }
  },
  "lift":       { "result": 18, "goal_achievement": 26, "environment": -3, "service": 2, "agent": 21 },
  "lift_total": { "result": 31, "goal_achievement": 43, "environment": 1, "service": 6, "agent": 32 },
  "confidence": "clear",
  "report_url": "https://sightmap.org/atlas/axis/airbnb/2026-08-20.html",
  "per_scenario": [
    { "key": "02-extract-price", "archetype": "extract", "no_context": 39, "with_skills": 54, "with_sightmap": 82, "lift": 28 }
  ]
}
```

Field notes:

- **Each rung** is the **median** across its `runs` repetitions per scenario,
  then the mean across scenarios. Median first because a single CAPTCHA or
  timeout should not define an entry. `no_context` runs at lower n by design: it
  is context for `lift_total`, not part of the headline instrument, and it is the
  least controlled rung (an agent in a bare workspace may or may not bootstrap
  its own tooling), so extra repetitions there buy noise, not precision.
- **`failed`** tallies, per rung, jobs that timed out or died; they are excluded
  from that rung's median.
- **`lift`** is `with_sightmap − with_skills`, per dimension. **`lift_total`** is
  `with_sightmap − no_context`. `lift` is the headline everywhere.
- **`spread`** is the interquartile range of the composite across repetitions. It
  is the honesty field: a lift of 6 with spreads of 20 is noise, and §6 refuses
  to publish it.
- **`entry_commit`** is the entry's commit at measurement time. §5 uses it to
  detect a score gone stale against edited content.
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
  "no_context": 48,
  "with_skills": 61,
  "with_sightmap": 79,
  "lift": 18,
  "lift_total": 31,
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
- All three rungs always travel together. Publishing `with_sightmap` alone would
  invite reading it as an absolute grade of the *site*, which it is not.

### 4.3 What `axis` is not

Not a rating of the mapped site. Not a rating of the agent. It is a measurement
of **one map's usefulness to one agent on one day**, and every field that
qualifies it — date, agent, model, harness, tier, confidence — travels with the
number everywhere it is shown.

---

## 5. Publication and display

Two standing rules govern everything below. **Per-dimension deltas are always
shown wherever a composite is** — §2.6's dilution means the composite alone
under-reports the dimensions this measurement is about. And **nothing branded
AXIS is published before Netlify has reviewed the methodology** (PLAN, post-spike
sync): these scores stretch the tool past its design centre, and doing that in
public under their name is a decision made with them, not about them.

**Card (`/atlas`).** One badge showing the headline lift — whether that number is
the composite lift or the Goal Achievement lift is decided post-spike with
Netlify (see §2.6 and PLAN's open questions) — coloured by confidence, not by
score band. Unscored entries show nothing — no placeholder, no "not yet scored" chip.
Sorting by lift is allowed; it must place unscored entries last rather than at
zero.

**Detail page (`/atlas/:slug`).** A scorecard section: the three rungs as a
ladder, the four per-dimension deltas for the headline lift (including negative
ones), `lift_total` clearly labelled as the whole-stack comparison,
agent/model/harness/date/tier, a link to the HTML report, and one sentence of
plain-language methodology linking to this spec. Negative dimension deltas render
exactly as measured.

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

1. `with_skills` and `with_sightmap` each completed ≥3 runs per scenario
   (target 5) on at least 3 scenarios. `no_context` needs only enough completions
   to report `lift_total`; when it has none, `lift_total` is omitted and the
   measurement stands.
2. Failure is tallied **per rung**. A job that timed out or died is excluded from
   its rung's median and counted in `failed`. If more than a third of
   `with_skills` or `with_sightmap` jobs failed, the measurement is void.
   Wholesale failure of `no_context` is not spoilage — an agent that cannot
   complete the task at all without tooling *is the status quo*, and is recorded
   as exactly that.
3. The judge adapter differs from the agent under test.
4. Dimension weights are AXIS defaults.
5. `|lift.result| > max(with_skills.spread, with_sightmap.spread)` →
   `confidence: "clear"`. Otherwise `weak`. Only `clear` is published as a
   headline; `weak` is written to `history/` and shown on the detail page as
   "within run-to-run noise."
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
circumvention" applies in full. A site that turns an automated Chrome away is
telling us something, and the answer is to record that and not score it — never
to work around it.

| Tier | What it is | Status |
|---|---|---|
| `live` | Read-only tasks against the public site, low concurrency, scheduled not per-merge, single region | supported at launch |
| `self-hosted` | The entry maps OSS we run ourselves; fully deterministic | supported at launch, preferred for anything used in methodology writing |
| `replay` | Scored against a frozen capture of the site | **not** in scope; reserved so the field does not have to change later |

Two consequences: demonstrate on `self-hosted` and on our own properties first,
where a bad number means a bad map rather than a bad afternoon on someone else's
CDN; and never gate an atlas contribution on a `live` score — see §8.

---

## 8. Non-goals

- **No admission gate.** A minimum lift is not required to merge an entry. Cost
  and `live` variance both make it unfair at launch, and it would push
  contributors toward maps that score rather than maps that are true. Revisit once
  there are enough measurements to know what the distribution even looks like.
- **No new CLI verb.** No `sightmap axis`. AXIS is the harness; reimplementing any
  part of it inside our CLI buys nothing and forks the numbers. The
  `with-sightmap` setup is already expressible with shipped verbs:
  `sightmap atlas add <slug>`, and `sightmap skills install --target` if AXIS's
  own `skills` field turns out not to reach the agent.
- **No spec change.** Nothing here touches `spec/v1/`, the YAML schema, or the
  meaning of any sightmap field. No SEP is required. The `index.json` extension in
  §4.2 is an atlas contract and lands in atlas `docs/SPEC.md`.
- **Not a site rating.** See §4.3. Any UI or copy that reads as grading the mapped
  site is a bug against this spec.
- **Track A is not in scope here.** See §2.5.
