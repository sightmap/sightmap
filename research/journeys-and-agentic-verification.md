# Journeys: verified user trajectories as a first-class Sightmap entity

> **Status:** research note, pre-SEP. Not normative. See [`README.md`](README.md).
> **Date:** 2026-08-29 · **Revised:** 2026-08-29 (rev 3)
> **Prompted by:** a conversation with a customer engineer on Gherkin,
> playwright-bdd, and agentic BDD, in the context of Subtext session review and
> Sightmap.

> **Rev 3** reconciles against Clint's prior art (the Sightmap Runtime Graph
> and App Areas designs). Six corrections, worked through in
> [`journeys-vs-runtime-graph-and-areas.md`](journeys-vs-runtime-graph-and-areas.md):
> the enricher blocker is narrower and worse than §2.2 claimed; the `expect:`
> dialect already exists in the areas proposal and should be inherited rather
> than reinvented; the step emitter already exists as `plan.py`; `requires:`
> synthesis makes a journey much smaller than §3.1 sketches; the coverage funnel
> in the index doc supersedes J1/J2/J3; and numeric comparison now blocks three
> proposals at once. The sections below carry those corrections inline.
>
> **Rev 2 changed four things** after review. The entity is renamed `journeys:`;
> rev 1 called it `flows:`, which collides with the WebMCP tool's own `flow:`
> step list. The bidirectional claim now carries its dependency: it needs an
> enricher, not just a file format. Stage 0 starts from the WebMCP step list
> plus a prose field instead of a Gherkin-keyed shape. And two examples were
> carrying the very glue layer the note argues against.

---

## The one-paragraph version

Sightmap is a map of **nouns**: views, components, requests, messages,
properties, memory. It has no **verbs**. Every consumer that needs sequence has
invented its own private verb layer. The WebMCP work invented `flow:` steps.
Subtext's review skill emits reproduction steps as throwaway prose. Playwright's
planner agent writes unanchored Markdown test plans. Cucumber has spent fifteen
years maintaining a hand-written glue layer between prose steps and selectors.
All four are trying to say *this sequence of interactions, against these parts
of the app, produces this outcome*, and none of them can say it in a form the
others can read. A `journeys:` entity, where each step carries readable prose
**and** a binding to named corpus entities, is that form. Compiled forward it
drives an executor. Read backward, **given a recorder that resolves
interactions against the same corpus**, it is a predicate you can match a
session stream against. The forward half is near-term and Sightmap-only. The
backward half is the bigger prize and it has a dependency, which §2.2 spells
out.

---

## 1. What exists today

### 1.1 The corpus (shipped)

`.sightmap/` names an app's structure and is curated by an agent against the
live DOM, with a coverage discipline that has a done signal (T3 = 0, T2
triaged). The parts this note builds on:

| Entity | Carries | Note |
|---|---|---|
| `views:` | `route` glob, `url`, `source`, `dependencies`, `tags`, `memory` | The unit of "where you are" |
| `components:` | `selector[]`, `children:`, `properties:`, `tags`, `$ref`, `stability` | Verified, shadow-flattened, source-linked |
| `requests:` | `route`, `method`, `properties:` (SEP-0005) | The unit of "what the app actually did" |
| `messages:` | `level`, `message` regex (SEP-0006) | Console/exception patterns |
| `signals:` | `ref:` + `filter:` → named, tagged classification (SEP-0007, PR #114) | A **point** classification composed from existing entities |
| `memory:` | freeform strings at file/view/component level | The hazard lore |

Two pieces of existing machinery do more than their docs suggest:

- **`dependencies:`** (SEP-0001). Minimatch globs declaring which source files,
  when changed, should trigger **re-curation** of a view or component. That is
  a change-impact index. Nothing consumes it for test selection yet.
- **`compquery`**. The component-query DSL (`Row[label="X"] Buy`, `#N`,
  descendant chains, `=`/`^=`/`*=` predicates), which addresses elements by
  *sightmap identity* instead of by selector or ephemeral node id. This is what
  a journey step should bind to, with a caveat about `#N` in §3.3.

The spec already flags the gap, in its own words:

> **Open questions** … Macros — learned trajectories that replay and heal when
> the site changes (not yet in the spec)
> — [`spec/v1/schema.md`](../spec/v1/schema.md)

### 1.2 The WebMCP branches (in review, PRs #281 / #292 / #293 / #294)

The most important prior art is in the repo already, and it is a step compiler
under another name.

`webmcp.tools.yaml` pairs a natural-language `description`, the agent-facing
contract, with a `flow:` list of steps that reference corpus entities **by
name**:

```yaml
- name: search_products
  description: >
    Keyword search over the IKEA US catalog. The summary count is the
    authoritative total — the grid renders only the first page of ~22 cards.
  mode: fetch
  view: SearchResults
  params: [{ name: query, type: string, required: true }]
  flow:
    - navigate: "/us/en/search/?q={query}"
    - read:
        summary: { component: SearchSummary, property: summary }
        products:
          for_each: ProductCard
          fields:
            price: { component: CardPrice, property: price_text }
```

The pipeline is `manifest + corpus → compile (resolve everything, error on
anything that doesn't) → IR → emit (three formats) → verify live → drift-gate
in CI (generate --check)`. Three properties of that design generalize:

1. **The manifest never contains a selector.** It names corpus entities and the
   compiler inlines them. A `css:` escape hatch exists and is treated as a smell.
2. **Compile-time resolution, errors not guesses.** Ambiguous component names
   list their breadcrumb candidates; a missing property lists what the
   component does declare.
3. **Provenance and drift.** The generated bundle carries a corpus content
   hash, so "the map moved and the artifact didn't" fails CI instead of
   becoming a mystery.

A WebMCP tool call dies with its document, so `navigate` is legal only as a
final step and cross-page journeys split into per-view tools gated by
`require_view:`. That is the WebMCP **executor's** constraint rather than a
flaw in the manifest, and the fact that the manifest had to model it at all
argues that a journey representation must be **executor-neutral** and sliced
per target.

One more thing to take from this file: its step list is called `flow:`. That is
why this note calls the corpus collection `journeys:`. See §3.4.

### 1.3 Subtext session review (shipped, adjacent product)

`review-open` returns a *map* (signal counts by kind and tag, page flow) over a
signal stream; `review-zoom` returns slices at four grains; `review-snapshot`
returns the screen. Sightmap enrichment is what gives those signals names. A
component's `tags:` rides onto any signal targeting it, and a `signals:` rule
mints a `classified` signal from a payload match.

Step 6 of the `subtext-review` skill is what this note is about:

> ### Reproduction steps
> 1. Navigate to \<URL\>
> 2. \<action\> — e.g., "Click the 'Sign in' button"
> 3. \<observation to confirm\> — e.g., "Verify modal appears within 2s"

That is a Gherkin scenario written in prose, produced on demand, anchored to
nothing, and discarded when the conversation ends. The system's most useful
output has nowhere to live.

### 1.4 The outside world

| Thing | What it gets right | What it can't do |
|---|---|---|
| **[Gherkin](https://cucumber.io/docs/gherkin/reference)** | Prose that a PM, a dev, and a parser can all read. `Scenario Outline` + `Examples` is parameterization done right. Tags. 70+ languages. | Steps bind to implementations through a **regex-matched glue layer of hand-written functions containing selectors**. That layer is where 100% of the rot lives. |
| **[playwright-bdd](https://github.com/vitalets/playwright-bdd)** | `bddgen` compiles `.feature` into real Playwright specs, so you keep the runner (auto-wait, parallelism, traces). Positions BDD specs as "AI-readable artifacts agents can verify against." | Still needs the step-definition layer. The generator's input is prose; nothing verifies a step corresponds to something in the app. |
| **[Cucumber BDD](https://cucumber.io/docs/bdd)** (discovery / formulation / automation) | Treats examples as the durable asset and the spec as *living documentation*. | Formulation is a workshop output. Nothing keeps it honest against the running app. |
| **[Playwright Test Agents](https://playwright.dev/docs/test-agents)** (planner / generator / healer) | Correct decomposition. Planner emits a Markdown plan; generator verifies selectors live while writing; healer replays and repatches. | The plan is unanchored Markdown, per-repo and per-tool. The healer heals **one test at a time, at runtime, invisibly**. Nothing is shared or reviewed. |
| **[Meticulous](https://www.meticulous.ai/how-it-works)** | Records real sessions, replays events against a build, diffs visual snapshots, mocks the backend, curates a session set for code coverage. Closes the prod-to-test loop, and has the recorder §2.2 says is required. | Tests are opaque event streams rather than readable specs. You can re-record intent but never edit it. Proprietary, hosted, and the artifact isn't yours. |
| **Stagehand / Midscene / Testim-class healers** | Runtime resilience is real and users like it. | Healing is per-test, unreviewable, and repeated N times for one UI change. The knowledge accrues nowhere. |
| **[Spec Kit](https://github.com/github/spec-kit) / Kiro (EARS)** | The galaxy-brain framing, productized: spec as unit of work, code as regenerable output, requirement-to-task traceability. | Specs are written ahead of the code and drift the moment they merge. Nothing derives them from observed behavior. |

Every row shares a shape. The industry has converged on "a human-readable flow
spec, plus an agent, plus a browser." Everyone has the prose. Nobody has a
**verified, versioned, portable binding from prose to app structure**, because
producing one requires the curated, coverage-scored, source-linked map that
Sightmap already builds.

---

## 2. The claims, and what each one depends on

### 2.1 Heal the map, not the test

*Depends on nothing that isn't already in the repo. This is the wedge.*

Every self-healing product on the market heals *the test*. A locator breaks, an
LLM looks at the DOM, guesses a new locator, patches that one test at runtime
without review. One UI change costs N heals across N tests, produces N opaque
decisions, and leaves nothing behind.

Sightmap inverts it. The selector lives in one place, the corpus. A PR touches
`src/components/DatePicker.tsx`; `dependencies:` says which views and components
are now stale; the authoring agent re-curates those against the live app with
the existing coverage loop; the selector changes **once, in a reviewed diff**.
Every journey bound to that component regenerates. So do the WebMCP tools. So
does session enrichment for every replay from now on.

One heal, reviewed, diffable, amortized across every consumer. That is the
entire open-source pitch, and it stands on its own.

### 2.2 One artifact, two directions

*True, but conditional. State the condition first.*

**Forward (executor).** `click: {component: AddToCartButton}` compiles, through
the existing corpus resolution, to a selector chain that a Playwright locator, a
WebMCP runtime action, or `sightmap browser click` can execute. This half needs
no other party.

**Backward (matcher).** The same line is a predicate over a signal stream in
which each interaction has *already* been resolved to a corpus component. Then
"did this session click AddToCartButton?" is a lookup rather than an inference.

Rev 1 said no other flow format can be read backwards and called it a moat that
"comes for free." Both halves of that were wrong.

The backward read is a property of the format *plus an enricher*: something
watching real usage that resolves each interaction against the same corpus.
Handed a raw click stream with no enrichment, a `journeys:` file is as blind as
a `.feature` file. And the only production-grade enricher today is Subtext,
which makes the backward direction a Sightmap + Subtext capability. It belongs
in Stage 3 (§7) rather than in the OSS pitch.

**Rev 3 correction.** An earlier draft proposed a one-day spike to find out
whether an enricher could exist outside Subtext. That was aimed at the wrong
question. Enrichment already exists: authored component and request tags are
unioned onto `Signal.Tags` in the pipeline today, and the runtime-graph design
specs a schema-aware `live-*` layer as "a stateless match pass over the loaded
sightmap, sitting between CDP output and the tool response." It is a match pass,
not a proprietary capability, and it degrades gracefully on partial coverage.

The real blockers are narrower and worse, and both are already documented:

1. **View tags don't reach the signal stream.** Enrichment consults components
   and requests only, and matches route-blind. A journey step keyed on `view:`
   cannot be matched backward until that changes.
2. **There is no org-level persisted corpus.** A corpus exists per session, via
   a single-use MCP upload. Matching a population needs it persisted
   server-side.

The second is infrastructure rather than a spike, it gates the areas proposal
identically, and it should be sequenced as shared work. The corpus matcher being
offline by design (`sightmap` never imports `browser`; `match.MatchTree` runs
over any serialized component tree) still suggests a non-Subtext recorder could
feed it, but that is a secondary question behind the persisted-corpus one.

So the format makes the backward read *possible*, the enrichment to do it
exists, and what's missing is where the corpus lives. That is a strong position
without the "nobody else can" that rev 1 claimed.

### 2.3 A spec derived from the running app is a spec you can't lie in

Spec Kit and Kiro write specs ahead of the code; they drift on merge, and the
drift is silent. A Sightmap journey is extracted from behavior, recorded from a
real browser session or a real production trajectory, with every binding
resolved against the live DOM at curation time and re-checked by CI. The
galaxy-brain regenerate-the-app-from-specs idea fails as an opaque abstraction,
but it works as a **delta**: start from a spec you extracted from the app as it
is, describe the change on top, let agents build and verify the difference.
Every other version of spec-driven development starts from a spec nobody has
checked.

---

## 3. The proposal sketch

### 3.1 Stage 0 shape: start here, not from Gherkin

The cheapest first version is **the WebMCP step list, lifted to a corpus
collection, plus one non-load-bearing prose field**. It reuses a compiler that
exists and commits to nothing the SEP would have to walk back.

```yaml
version: 1

journeys:
  - name: SearchAndAddToCart
    description: Find a product by keyword and add it to the basket
    tags: [revenue, smoke, p0]
    dependencies: ['src/features/search/**']
    params:
      - { name: query, type: string, example: "desk chair" }
      - { name: article, type: string, example: "40504193" }

    steps:
      - prose: I search for "{query}" from the search page
        view: SearchResults
        act:
          - navigate: "/us/en/search/?q={query}"
          - fill:  { component: SearchInput, value: "{query}" }
          - click: { component: SearchButton }
        await:  { request: SearchAPI }
        expect:
          - { exists: ProductCard }
          - { signal: search.returned_results }

      - prose: I open the product I was looking for and add it to the cart
        view: ProductDetail
        act:
          - click: { component: 'ProductCard[article="{article}"]' }
          - click: { component: AddToCartButton }
        await:  { request: AddToCart }
        expect:
          - { request: AddToCart, status: 200 }
          - { signal: cart.item_added }
        not_expect:
          - { signal: checkout.payment.declined }
```

One `prose:` string per step, documentation only, and nothing resolves by
matching it. Everything else is the WebMCP vocabulary (`navigate` / `fill` /
`click` / `read`) plus `await` and `expect`.

**Rev 3: two corrections to this shape.**

*Inherit the `expect:` dialect, don't invent one.* The areas proposal already
defines `expect: { path: [...], requests: {Name: {status}}, never: [...] }`,
where `path` is an ordered view subsequence that tolerates detours and
`requests:` reuses SEP-0007's `filter:` verbatim. That is the same matching
semantics §6 arrives at independently, already scoped against a real corpus.
Journeys should take it unchanged and extend only where a journey needs more
than an area does: assertions at step granularity rather than per surface, and
component bindings between the view waypoints. `not_expect:` above is that
proposal's `never:` under a worse name.

*The steps get shorter.* The runtime-graph planner synthesizes mechanical steps
from a component's `requires:` clauses — `field_filled: X` becomes `fill X`,
`field_filled_and_blurred: X` becomes `fill X` then `blur X`. A journey that
spells out every fill and blur restates what the graph derives. Name the
waypoints and the assertions; let the planner fill in the mechanics. The example
above is roughly twice as long as it needs to be against a corpus that carries
edges.

### 3.2 Gherkin keys are a Stage 2 question

Rev 1 opened with `given:` / `when:` / `then:` keys alongside `act:` / `await:`
/ `expect:` on every step. Two grammars on one node is awkward to specify,
validate and align against a signal stream, and it buys a `.feature` round-trip
nobody has asked for.

Prove the round-trip is wanted before paying for it. If a Cucumber team wants
`.feature` output, an emitter can derive it from `prose:` plus step position:
the first step's prose becomes `Given`, subsequent `act` steps become `When`,
and `expect:` entries render as `Then` clauses. That mapping is mechanical and
doesn't need to live in the corpus at all.

### 3.3 Two things rev 1 got wrong

**`matches: '\d+ results'` smuggled the glue layer back in.** A regex over
extracted text is app knowledge living in the assertion, which is what "no
selectors in the journey" is supposed to prevent, one level up. The rule
generalizes: an assertion names a thing the corpus already extracted and never
re-derives it.

There is a real gap underneath. SEP-0010 **removed `transform`**, so a component
property extracts raw text and there is no way to declare "the integer inside
`42 results`". Three options, in preference order:

1. **A `signals:` rule** (SEP-0007) with a `filter:` on the extracted property.
   This is what signals are for, and `expect: {signal: …}` stays clean.
2. **`exists:`**, which is often the assertion that matters ("results came back
   at all") and needs nothing new.
3. **A typed comparison on a property**, which means a new SEP. Don't reach for
   this to unblock journeys.

**`ProductCard#1` is a replay hazard.** Ordinal addressing is fine for a
synthetic smoke test that seeded its own data. In a production-derived journey
it is a trap: search ranking changes, the first card is a different product, the
assertion fails, and the failure reads as a product bug. A new suite loses
credibility fastest through false failures that look real.

So a journey extracted from a real session **binds by property predicate**
(`ProductCard[article="{article}"]`), with the discriminating value lifted into
`params:`. If the corpus can't discriminate the instance, that is a corpus gap;
fix it there, the way the authoring skill already treats an ambiguous component
query. `#N` stays legal for hand-authored synthetic journeys and should be
lint-warned in extracted ones.

### 3.4 The name

Rev 1 called this `flows:`, which collides with the WebMCP tool manifest's own
`flow:`, the step list on a single tool. Two different things behind one word,
in the same repo, in skills and compiler error messages that already mention
both. The concept survives; the identifier doesn't.

| Candidate | For | Against |
|---|---|---|
| **`journeys:`** *(recommended)* | No collision. "User journey" is standing industry vocabulary a PM already owns. Reads correctly for both a test and an observed trajectory. | Slightly long; no precedent in the spec text. |
| `macros:` | The spec's own open question uses this word, so there's provenance. | "Macro" implies replay and automation rather than specification, and it's a loaded term (Excel, editor macros). Undersells the artifact. |
| `trajectories:` | Precise, and matches how the agent-evals world talks. | Jargon for a PM; long; reads analytical rather than intentional. |

Recommend `journeys:`. Say in the SEP why not `macros:`, since that open
question is the thing being resolved and deserves an explicit answer.

---

## 4. The loop this closes, and which parts are Stage 3

```
  ┌──── STAGE 3 · needs an enricher (Subtext today) · §2.2 ────────────────┐
  │                                                                        │
  │  production ──► enriched signal stream ──► journey matcher             │
  │  sessions                                        │                     │
  │                    ┌─────────────────────────────┼──────────────┐      │
  │                    ▼                             ▼              ▼      │
  │            matched + passed            matched + failed@N   unmatched  │
  │                    │                             │              │      │
  │          frequency evidence          reproduction, anchored  NEW TRAJ. │
  │          (informs pruning —          at a step boundary      (corpus   │
  │           does NOT outrank                   │                gap +    │
  │           p0/keep tags §4.1)                 │              candidate) │
  └────────────────────┼─────────────────────────┼────────────────┼───────┘
                       │                         ▼                │
                       │              regression journey ─────────┘
                       ▼                         │
              ┌─────────────────────────────────────────────┐
              │   .sightmap/  views · components · journeys  │  ◄── STAGE 1
              └─────────────────────────────────────────────┘
                       │
      ┌────────────────┼──────────────┬──────────────────┐   ◄── STAGE 2
      ▼                ▼              ▼                  ▼
  Playwright      WebMCP tools   agent step-lists   journey coverage
  spec files      (existing!)    (token-cheap        report
                                  pre-scripted
                                  sequences)
```

Everything below the corpus box is Sightmap-only and near-term. Everything above
it needs the enricher from §2.2. Pursue both halves together; pitch them
separately.

### 4.1 Production to corpus, the Subtext trigger (Stage 3)

A session's enriched signal stream is aligned against every known journey. Three
outcomes:

- **Matched, succeeded.** Frequency evidence. "`SearchAndAddToCart` ran 4,102×
  this week; `AdminBulkExport` ran twice."

  Rev 1 called this "the only honest input to which tests are worth
  maintaining," which is wrong and dangerously so. **Traffic is a weak proxy for
  value.** Admin tools, refund and cancellation paths, accessibility routes,
  data-export and compliance flows, first-run onboarding: all low-traffic by
  construction, and several of them are the ones you least want to discover
  broken. A suite pruned by traffic converges on the happy path and quietly
  abandons everything else.

  So frequency informs pruning and never decides it. A `p0` or `keep` tag
  outranks traffic outright. A low-traffic journey with no such tag surfaces as
  a question ("nothing has exercised this in 90 days, still real?") rather than
  a deletion. And zero traffic on a journey that should be busy is a finding in
  its own right.

- **Matched, failed at step N.** A reproduction anchored at a step boundary,
  with the discriminating parameter values, privacy-scrubbed per §6. Not a prose
  repro: a regression journey with a known-good prefix and a known-bad step,
  ready to run in CI. Binding is by property predicate per §3.3, since an
  ordinal-bound extraction would produce false failures.

- **Matched nothing.** An unknown trajectory, which is two things at once: a
  candidate new journey, and usually a corpus gap, because unmatched
  interactions cluster in T3/orphan territory. It feeds the authoring queue the
  coverage loop already maintains.

### 4.2 Corpus to artifacts (Stage 2)

Reuse the WebMCP compiler's architecture verbatim: `journeys + corpus → resolve
everything → IR → emit`. Only the emitters differ.

- **`--emit playwright`.** A spec file per journey. The playwright-bdd insight
  (keep the real runner, generate into it) without the step-definition layer,
  because the bindings are the step definitions and they're declarative.
- **`--emit webmcp`.** The existing generator, with journeys sliced at
  `navigate` boundaries into per-view tools gated by `require_view:`. The
  slicing rule is already implemented; journeys become a second front end to it.
- **`--emit steps`.** A compact, pre-resolved step list an agent executes with
  the `sightmap-browser` tools: the "pre-scripted sequences of browser
  interactions an agent can piece together to save tokens and increase
  reliability" from the original conversation. **Rev 3: this already exists.**
  The runtime-graph work ships `plan.py`, validated by playback in a real
  Subtext-Local session, with measured token math (roughly 12 round-trips
  collapsing to one plan plus one verification snapshot) and a proposed
  `sightmap-plan(from, goal, params)` MCP tool. Journeys should call the
  planner, not reimplement it — a journey names the waypoints and the planner
  produces the steps between them.
- **`--emit feature`.** A `.feature` file, derived per §3.2, if the round-trip
  proves wanted.

`generate --check` gates drift on all of them, as it does for WebMCP bundles
today.

### 4.3 SDLC triggers

| Trigger | What runs | Stage |
|---|---|---|
| **Per-PR** | Changed files → `dependencies:` globs → stale views/components → re-curate those → regenerate their bound journeys → run only those | 4, though the index exists today |
| **Nightly cron** | Full re-curation sweep, full journey suite, drift report | 2 |
| **On Subtext session review** | §4.1: file a corpus gap, a candidate journey, or a regression journey | 3, needs the enricher |
| **On corpus merge** | Regenerate every artifact; fail on drift | 2 |

### 4.4 Journey coverage, a behavioral tier ladder

Today's coverage is **spatial**: of the interactive nodes on this page, how many
resolve to a named component (T1/T2/T3)? The sibling metric is **behavioral**:
of the trajectories users actually perform, how many does a journey describe?

**Rev 3: use the index doc's funnel, not a parallel ladder.** The App Areas work
already defines this and specifies it better:

```
114 raw signals
 → 112 matched a sightmap entity          98%  (named)
 →  90 that entity is an area member      79%  (attributed)
```

Two numbers, both deterministic, both computable with no LLM, and the second can
never exceed the first. Journey coverage is a third rung on that funnel — *of the
attributed activity, how much lies on a declared journey?* — not a new
vocabulary. The rev-2 J1/J2/J3 tiers are withdrawn; the distinction they were
reaching for (full match versus prefix match versus none) belongs inside the
third rung as a breakdown, not as a peer metric to T1/T2/T3.

Whatever it is called, it is **Stage 3 and blocked on the persisted corpus** from
§2.2, so it stays out of the near-term open-source story.

---

## 5. Why this is the right open-source wedge

Sightmap's OSS position today is "an agent-readable map of your app." Good, but
narrow: it reads as valuable mostly to teams already running coding agents
against a web app.

Sell **§2.1 plus §4.2: heal the map, and compile it into tests.** Both are
Sightmap-only, both are near-term, and together they're a complete story that
never mentions session replay:

> Your E2E tests break because selectors live in your tests. Move them into a
> map that an agent maintains against your running app, write your journeys
> against that map, and generate the tests. When the UI changes, the map heals
> once, in a PR you review, and every test that touched it heals with it. The
> journeys are YAML in your repo, so you can leave whenever you want.

The production loop (§4.1) is the bigger prize and the better demo, but it is
Stage 3 and carries the §2.2 dependency. Leading with it would put a capability
that needs Subtext at the center of an open-source pitch, and teams would find
the gap after adopting. Lead with heal-and-compile, and let the loop be what
gets unusually good if you also run an enricher.

What isn't open today, and is worth claiming:

- Gherkin is prose-only and its glue layer is per-repo, per-language, hand-written.
- Playwright's test plans are Markdown, per-tool, unversioned, unanchored.
- Every AI-testing vendor (Meticulous, Testim, Momentic, QA Wolf, Autify) has a
  proprietary internal flow representation you cannot export, diff, or leave with.

**There is no open, portable, executor-neutral, machine-checkable format for a
verified user journey.** That hole sits directly on top of what Sightmap already
builds, and filling it pulls in contributors from the Playwright and Cucumber
communities rather than only from the AI-agent one.

---

## 6. Problems

1. **Sequence matching is fuzzy, and pretending otherwise will sink it.** Real
   users interleave, retry, and abandon. Step equality is the wrong primitive.
   The pragmatic v1 is **ordered-subsequence matching with a gap budget**, with
   J2 first-class rather than a failure mode.
2. **The backward read has a dependency (§2.2).** Stating it late, or letting a
   diagram imply otherwise, is the most damaging thing this proposal could do to
   its own credibility.
3. **Ordinal binding produces false failures (§3.3).** Extracted journeys bind
   by property predicate, and `#N` needs a lint rule rather than a convention.
4. **Numeric comparison blocks three proposals at once.** SEP-0010's removal of
   `transform` leaves a real gap for value-shaped assertions (§3.3), SEP-0007
   deferred comparison operators, and the areas proposal needs them too ("no
   more than two password attempts", "under 8 seconds"). The rev-1
   `matches: '\d+ results'` mistake was this gap in disguise, and the three
   options in §3.3 are all workarounds for its absence. Resolve it once, across
   proposals, before either areas or journeys drafts.
5. **Privacy is first-order, not a footnote.** Production trajectories carry PII
   in the exact places journey params live, and §3.3's fix makes it sharper:
   binding by predicate means the discriminating value is the thing you capture.
   Extraction runs **behind** the recorder's privacy rules, bindings stay
   structural by default, and captured `example:` values are opt-in and
   scrubbed. Getting this wrong once would be worse than not shipping.
6. **Pruning by traffic deletes rare-but-critical paths (§4.1).** Tags outrank
   frequency; frequency raises questions and never deletes.
7. **Journeys are a new maintenance surface.** Fifty that nobody curates is a
   worse artifact than five that are current, and #6 means the obvious automatic
   remedy is the dangerous one.
8. **`signals:` (SEP-0007) is not merged.** `expect: {signal: …}` depends on it,
   and §3.3 makes signals the preferred answer for value assertions, which
   deepens the dependency rather than softening it. Journeys can ship on
   components, requests, views and `exists:` alone. State the dependency instead
   of discovering it.
9. **Scope discipline.** One SEP, one decision: the entity. Emitters, the
   session matcher, and journey coverage are follow-on work.

---

## 7. A staged plan

**Stage 0: `sightmap journey record` / `journey run` (no spec change).**
Drive the browser with the existing CLI, emit a journey YAML in the §3.1 shape
(WebMCP step vocabulary plus one `prose:` string), run it back with the existing
browser primitives. Ships as reserved tooling; the spec has a `Reserved tooling
fields` escape hatch for exactly this. The purpose is to find out whether the
shape survives contact with three real apps before anyone writes an SEP. Don't
start from a Gherkin-keyed shape (§3.2).

**Stage 1: the SEP, a `journeys:` entity.**
One decision: the entity, its steps, its bindings, its resolution rules, and why
`journeys:` and not `macros:`. Don't announce a number while floating the idea.
The process claims a number at draft-PR time, and pre-claiming in a Discussion
is how two proposals collide. `0011` is free today (0001–0010 on `main`, 0008
claimed by open PR #166) and may not be when the PR opens.

**Stage 2: emitters.**
Generalize the WebMCP compiler's `compile → IR → emit` spine to a second front
end. `--emit playwright` and `--emit steps` first. `--emit feature` once someone
asks for the round-trip.

**Stage 3: the session matcher (blocked on shared infrastructure).**
Align an enriched signal stream against corpus journeys; emit
`journey.matched` / `journey.failed_at_step` / `journey.unmatched`. Frame it as
"`signals:` with sequence": SEP-0007 classifies a point, this classifies a path.
Gated on the two blockers in §2.2 — view tags reaching the signal stream, and an
org-level persisted corpus — both of which gate the areas proposal identically
and should be sequenced as shared work rather than discovered twice.

**Stage 4: coverage and CI.**
`sightmap journey coverage` (J1/J2/J3 against a session set),
`dependencies:`-driven journey selection per PR, drift gates.

---

## 8. What to do next

1. **Float it.** A Discussion under Ideas, per the SEP process. This note is the
   float material, no SEP number attached.
2. **Stage 0, against three apps.** The docs site's own `.sightmap/`, the IKEA
   atlas corpus already vendored for the WebMCP examples, and one real customer
   journey. Three journeys each, in the §3.1 shape. If it holds, write the SEP.
   If it doesn't, this note cost a week.
3. **Sequence the shared blocker** with the areas proposal: org-level persisted
   corpus, and view tags reaching the signal stream. Neither proposal should
   discover it independently.
4. **Land the WebMCP branches** (#281, #292, #293, #294). They are the compiler
   this depends on.
5. **Answer the privacy question in writing before Stage 3.** Not after.

---

## References

- Gherkin reference — <https://cucumber.io/docs/gherkin/reference>
- Cucumber BDD (discovery / formulation / automation) — <https://cucumber.io/docs/bdd>
- Cucumber Messages (NDJSON envelopes, AST + pickles) — <https://github.com/cucumber/messages>
- playwright-bdd — <https://github.com/vitalets/playwright-bdd>
- Playwright Test Agents (planner / generator / healer) — <https://playwright.dev/docs/test-agents>
- Meticulous — <https://www.meticulous.ai/how-it-works>
- Stagehand — <https://www.stagehand.dev/>
- GitHub Spec Kit — <https://github.com/github/spec-kit>
- WebMCP (W3C Web Machine Learning CG) — <https://github.com/webmachinelearning/webmcp>
- In-repo: [`spec/v1/schema.md`](../spec/v1/schema.md) · [SEP-0007 signals](../spec/seps/0007-signals.md) · [SEP-0010 tree-closed properties](../spec/seps/0010-tree-closed-component-properties.md) · [coverage model](../go/docs/reference/coverage-model.md) · [outer loop](../go/docs/reference/outer-loop.md) · [architecture](../go/ARCHITECTURE.md)
