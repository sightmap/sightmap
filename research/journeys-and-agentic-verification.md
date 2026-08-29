# Journeys: verified user trajectories as a first-class Sightmap entity

> **Status:** research note, pre-SEP. Not normative. See [`README.md`](README.md).
> **Date:** 2026-08-29 · **Revised:** 2026-08-29 (rev 2)
> **Prompted by:** a conversation with a customer engineer on Gherkin,
> playwright-bdd, and agentic BDD, in the context of Subtext session review +
> Sightmap.

> **Rev 2 changed four substantive things** after review. The entity is renamed
> `journeys:` (rev 1 called it `flows:`, which collides with the WebMCP tool's
> own `flow:` step list). The bidirectional claim is now stated with its
> dependency — it needs an enricher, not just a file format. Stage 0 starts
> from the WebMCP step list plus a prose field rather than from a Gherkin-keyed
> shape. And the assertion and addressing examples were carrying the very glue
> layer the note argues against.

---

## The one-paragraph version

Sightmap is a map of **nouns** — views, components, requests, messages,
properties, memory. It has no **verbs**. Every consumer that needs sequence is
currently inventing its own private verb layer: the WebMCP work invented
`flow:` steps, Subtext's review skill emits reproduction steps as throwaway
prose, Playwright's planner agent writes unanchored Markdown test plans, and
Cucumber has spent fifteen years maintaining a hand-written glue layer between
prose steps and selectors. All four are trying to say the same thing —
*this sequence of interactions, against these parts of the app, produces this
outcome* — and none of them can say it in a form the others can read. A
`journeys:` entity in the corpus, where each step carries readable prose **and**
a binding to named corpus entities, is that form. Compiled forward it drives an
executor; read backward — **given a recorder that resolves interactions to the
same corpus** — it is a predicate a session stream can be matched against. The
forward half is the near-term, Sightmap-only work. The backward half is the
larger prize and it has a dependency; §2.2 is explicit about which is which.

---

## 1. What exists today

### 1.1 The corpus (shipped)

`.sightmap/` names an app's structure and is curated by an agent against the
live DOM, with a coverage discipline that has a done signal (T3 = 0, T2
triaged). What matters for this note:

| Entity | Carries | Note |
|---|---|---|
| `views:` | `route` glob, `url`, `source`, `dependencies`, `tags`, `memory` | The unit of "where you are" |
| `components:` | `selector[]`, `children:`, `properties:`, `tags`, `$ref`, `stability` | Verified, shadow-flattened, source-linked |
| `requests:` | `route`, `method`, `properties:` (SEP-0005) | The unit of "what the app actually did" |
| `messages:` | `level`, `message` regex (SEP-0006) | Console/exception patterns |
| `signals:` | `ref:` + `filter:` → named, tagged classification (SEP-0007, PR #114) | A **point** classification composed from existing entities |
| `memory:` | freeform strings at file/view/component level | The hazard lore |

Two pieces of existing machinery matter more than they look:

- **`dependencies:`** (SEP-0001) — minimatch globs declaring which source files,
  when changed, should trigger **re-curation** of a view or component. This is
  already a change-impact index. Nothing consumes it for test selection yet.
- **`compquery`** — the component-query DSL (`Row[label="X"] Buy`, `#N`,
  descendant chains, `=`/`^=`/`*=` predicates). Addressing by *sightmap
  identity* rather than by selector or ephemeral node id. This is the thing a
  journey step should bind to — with a caveat about `#N` in §3.3.

And the spec already flags the gap, in its own words:

> **Open questions** … Macros — learned trajectories that replay and heal when
> the site changes (not yet in the spec)
> — [`spec/v1/schema.md`](../spec/v1/schema.md)

### 1.2 The WebMCP branches (in review, PRs #281 / #292 / #293 / #294)

This is the most important prior art in the repo, and it is *already* a step
compiler — it just doesn't know it yet.

`webmcp.tools.yaml` pairs a natural-language `description` (the agent-facing
contract) with a `flow:` list of steps that reference corpus entities **by
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
in CI (`generate --check`)`. Three properties of that design generalize
directly:

1. **The manifest never contains a selector.** It names corpus entities; the
   compiler inlines. A `css:` escape hatch exists and is treated as a smell.
2. **Compile-time resolution, errors not guesses.** Ambiguous component names
   list their breadcrumb candidates; a missing property lists what the
   component does declare.
3. **Provenance + drift.** The generated bundle carries a corpus content hash,
   so "the map moved and the artifact didn't" is a CI failure, not a mystery.

The existing constraint that a WebMCP tool call *dies with its document* — so
`navigate` is legal only as a final step, and cross-page journeys must be split
into per-view tools gated by `require_view:` — is worth noticing. It is not a
flaw to design around; it is the WebMCP **executor's** constraint, and the fact
that the manifest already had to model it is evidence that a journey
representation must be **executor-neutral** and sliced per target.

**Note the name.** This entity's step list is called `flow:`. That is why this
note calls the corpus collection `journeys:` — see §3.4.

### 1.3 Subtext session review (shipped, adjacent product)

`review-open` returns a *map* (signal counts by kind and tag, page flow) over a
signal stream; `review-zoom` returns slices at four grains; `review-snapshot`
returns the screen. Sightmap enrichment is what gives those signals names —
a component's `tags:` rides onto any signal targeting it, and a `signals:` rule
mints a `classified` signal from a payload match.

The `subtext-review` skill's Step 6 is the thing this note is really about:

> ### Reproduction steps
> 1. Navigate to \<URL\>
> 2. \<action\> — e.g., "Click the 'Sign in' button"
> 3. \<observation to confirm\> — e.g., "Verify modal appears within 2s"

That is a Gherkin scenario, written in prose, produced on demand, anchored to
nothing, and discarded when the conversation ends. It is the single highest-value
artifact the whole system produces and it currently has no home.

### 1.4 The outside world

| Thing | What it gets right | What it can't do |
|---|---|---|
| **[Gherkin](https://cucumber.io/docs/gherkin/reference)** | Prose that a PM, a dev, and a parser can all read. `Scenario Outline` + `Examples` is parameterization done right. Tags. 70+ languages. | Steps bind to implementations through a **regex-matched glue layer of hand-written functions containing selectors**. That layer is where 100% of the rot lives. |
| **[playwright-bdd](https://github.com/vitalets/playwright-bdd)** | `bddgen` compiles `.feature` → real Playwright specs, so you keep the runner (auto-wait, parallelism, traces). Explicitly positions BDD specs as "AI-readable artifacts agents can verify against." | Still needs the step-definition layer. The generator's input is prose; nothing verifies a step actually corresponds to something in the app. |
| **[Cucumber BDD](https://cucumber.io/docs/bdd)** — discovery / formulation / automation | The insight that examples are the durable asset and the spec is *living documentation*. | Formulation is a workshop output. Nothing keeps it honest against the running app. |
| **[Playwright Test Agents](https://playwright.dev/docs/test-agents)** (planner / generator / healer) | Correct decomposition. Planner emits a Markdown plan; generator verifies selectors live while writing; healer replays and repatches. | The plan is unanchored Markdown, per-repo and per-tool. The healer heals **one test at a time, at runtime, invisibly**. Nothing is shared or reviewed. |
| **[Meticulous](https://www.meticulous.ai/how-it-works)** | Records real sessions, replays events against a build, diffs visual snapshots, mocks the backend, curates a session set for code coverage. Genuinely closes the prod→test loop, and it has the recorder this note's §2.2 says is required. | Tests are opaque event streams, not readable specs. You cannot *edit the intent*, only re-record. Proprietary, hosted, and the artifact isn't yours. |
| **Stagehand / Midscene / Testim-class healers** | Runtime resilience is real and users like it. | Healing is per-test, unreviewable, and repeated N times for one UI change. The knowledge never accrues anywhere. |
| **[Spec Kit](https://github.com/github/spec-kit) / Kiro (EARS)** | The galaxy-brain framing, productized: spec as unit of work, code as regenerable output, requirement→task traceability. | Specs are written **ahead of** the code and drift the moment they're merged. Nothing derives them from observed behavior. |

**The pattern across every row:** the industry has converged on "a
human-readable flow spec, plus an agent, plus a browser." Everyone has the
prose. Nobody has a **verified, versioned, portable binding from prose to app
structure** — because producing one requires exactly the curated, coverage-
scored, source-linked map that Sightmap already builds.

---

## 2. The claims, and what each one depends on

### 2.1 Heal the map, not the test — *no dependency, this is the wedge*

Every self-healing product on the market heals *the test*: a locator breaks,
an LLM looks at the DOM, guesses a new locator, patches that one test, at
runtime, without review. One UI change costs N heals across N tests, produces
N opaque decisions, and leaves nothing behind.

Sightmap inverts it. The selector lives in exactly one place — the corpus. A PR
touches `src/components/DatePicker.tsx`; `dependencies:` says which views and
components are now stale; the authoring agent re-curates *those* against the
live app with the existing coverage loop; the selector changes **once, in a
reviewed diff**. Every journey bound to that component regenerates. So do the
WebMCP tools. So does session enrichment for every replay from now on.

One heal, reviewed, diffable, amortized across every consumer. **This claim
needs nothing that isn't already in the repo or in review.** It is the whole
open-source pitch on its own, and it should be sold on its own.

### 2.2 One artifact, two directions — *true, but conditional; say the condition*

*Forward (executor):* `click: {component: AddToCartButton}` compiles — via the
existing corpus resolution — to a selector chain that a Playwright locator, a
WebMCP runtime action, or `sightmap browser click` can execute. **This half is
Sightmap-only and needs no other party.**

*Backward (matcher):* the same line is a predicate over a signal stream in
which each interaction has *already* been resolved to a corpus component. Then
"did this session click AddToCartButton?" is a lookup rather than an inference.

The rev-1 version of this note said "no other flow format can be read
backwards" and called it a moat that "comes for free." Both halves of that were
wrong:

- **It is not a property of the format.** It is a property of *the format plus
  an enricher* — something that observes real usage and resolves each
  interaction against the same corpus. Handed a raw click stream with no
  enrichment, a `journeys:` file is exactly as blind as a `.feature` file.
- **It is not free.** Today the only production-grade enricher is Subtext.
  That makes the backward direction a Sightmap + Subtext capability, not a
  Sightmap capability, and it belongs in Stage 3 (§7), not in the OSS pitch.

Two things keep it from being purely proprietary, and both are worth testing
rather than asserting:

1. The corpus matcher is **offline** by design (`sightmap` never imports
   `browser`; `match.MatchTree` runs over any serialized component tree). So
   *any* recorder that can produce component trees plus an interaction log —
   an rrweb capture, a Playwright trace, `sightmap flow record` from Stage 0 —
   is in principle enough to match backward without Subtext. Unproven; worth a
   spike, because it decides whether the loop is an open capability or a
   commercial one.
2. Meticulous demonstrates the recorder half is buildable and that customers
   will install a snippet for it. What they don't have is the readable, editable
   spec on the other end.

**Honest framing:** the format makes the backward read *possible*, and Sightmap
+ Subtext is the first place it's *practical*. That is still a strong position.
It is not "nobody else can."

### 2.3 A spec derived from the running app is a spec you can't lie in

Spec Kit and Kiro write specs ahead of the code; they drift on merge, and the
drift is silent. A Sightmap journey is *extracted from behavior* — recorded
from a real browser session or a real production trajectory, with every binding
resolved against the live DOM at curation time and re-checked by CI. The
"galaxy-brain" regenerate-the-app-from-specs idea fails as an opaque
abstraction, but it works fine as a **delta**: start from a spec you extracted
from the app as it is, describe the change on top, let agents build and verify
the difference. That is the only version of spec-driven development where the
starting spec is known to be true.

---

## 3. The proposal sketch

### 3.1 Stage 0 shape — start here, not from Gherkin

The cheapest first version is **the WebMCP step list, lifted to a corpus
collection, plus one non-load-bearing prose field**. It reuses a compiler that
already exists, and it commits to nothing the SEP would have to walk back.

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

One `prose:` string per step, and it is documentation only — nothing resolves
by matching it. Everything else is the WebMCP vocabulary (`navigate` / `fill` /
`click` / `read`) plus `await` and `expect`.

### 3.2 Gherkin keys are a Stage 2 question, not a Stage 0 one

Rev 1 opened with `given:` / `when:` / `then:` keys alongside `act:` /
`await:` / `expect:` on every step. That is two grammars on one node — awkward
to specify, awkward to validate, and awkward to align against a signal stream,
for a benefit (round-trip to `.feature`) that nobody has yet asked for.

So: **prove the round-trip is wanted before paying for it.** If a team on
Cucumber wants `.feature` output, an emitter can derive it from `prose:` plus
step position — the first step's prose becomes `Given`, subsequent `act`
steps become `When`, and `expect:` entries render as `Then` clauses. The
mapping is mechanical and doesn't need to live in the corpus at all. That is
strictly better than encoding Gherkin's shape in the entity on day one.

### 3.3 Two things in the rev-1 sketch were quietly wrong

**`matches: '\d+ results'` smuggled the glue layer back in.** A regex over
extracted text is app knowledge living in the assertion — precisely what "no
selectors in the journey" is supposed to prevent, one level up. The rule
generalizes: *an assertion should name a thing the corpus already extracted,
never re-derive it.*

There is a real gap underneath, and it should be stated rather than papered
over: SEP-0010 **removed `transform`**, so a component property extracts raw
text. There is currently no way to declare "the integer inside `42 results`".
Three honest options, in preference order:

1. **A `signals:` rule** (SEP-0007) with a `filter:` on the extracted property
   — this is exactly what signals are for, and `expect: {signal: …}` stays clean.
2. **`exists:`** — often the assertion that actually matters ("results came
   back at all") and it needs nothing new.
3. **A typed comparison on a property**, which would mean a new SEP. Don't
   reach for this to unblock journeys.

**`ProductCard#1` is a replay hazard.** Ordinal addressing is fine for a
synthetic smoke test that seeded its own data. For a production-derived journey
it is a trap: search ranking changes, the first card is a different product,
the assertion fails, and the failure *looks like a product bug*. Nothing is
more corrosive to a new test suite than false failures that read as real ones.

The rule: **a journey extracted from a real session binds by property
predicate** (`ProductCard[article="{article}"]`), with the discriminating value
lifted into `params:`. If the corpus can't discriminate the instance, that is a
corpus gap — fix it there, the same way the authoring skill already treats an
ambiguous component query. `#N` stays legal for hand-authored synthetic
journeys and should be lint-warned in extracted ones.

### 3.4 The name

Rev 1 called this `flows:`. That collides with the WebMCP tool manifest's own
`flow:` — the step list on a single tool. Two different things named the same
word, in the same repo, in skills and compiler error messages that already
mention both. The concept survives; the identifier doesn't.

| Candidate | For | Against |
|---|---|---|
| **`journeys:`** *(recommended)* | No collision. "User journey" is standing industry vocabulary a PM already owns. Reads correctly for both a test and an observed trajectory. | Slightly long; no precedent in the spec text. |
| `macros:` | The spec's own open question uses this word, so there's provenance. | "Macro" implies replay/automation, not specification — and it's a loaded term (Excel, editor macros). Undersells the artifact. |
| `trajectories:` | Precise, and matches how the agent-evals world talks. | Jargon for a PM; long; reads analytical rather than intentional. |

Recommend `journeys:`, and say in the SEP why not `macros:` — that open
question is the thing being resolved, so it deserves an explicit answer.

---

## 4. The loop this closes — *and which parts are Stage 3*

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

Everything below the corpus box is Sightmap-only and near-term. Everything
above it needs the enricher from §2.2. The two halves are worth pursuing
together and worth *pitching* separately.

### 4.1 Production → corpus (the Subtext trigger) — Stage 3

A session's enriched signal stream is aligned against every known journey.
Three outcomes:

- **Matched, succeeded.** Frequency evidence. "`SearchAndAddToCart` ran 4,102×
  this week; `AdminBulkExport` ran twice."

  Rev 1 called this "the only honest input to which tests are worth
  maintaining." That is wrong, and dangerously so. **Traffic is a weak proxy
  for value.** Admin tools, refund and cancellation paths, accessibility
  routes, data-export and compliance flows, first-run onboarding — all are
  low-traffic by construction and several are the ones you least want to
  discover broken. A suite pruned by traffic alone converges on the happy path
  and quietly abandons everything else.

  So: frequency *informs* pruning and never decides it. A `p0` or `keep` tag
  outranks traffic outright; a low-traffic journey with no such tag surfaces
  as a *question* ("nothing has exercised this in 90 days — still real?"), not
  as a deletion. And zero traffic on a journey that should be busy is itself a
  finding worth surfacing.

- **Matched, failed at step N.** A reproduction **anchored at a step boundary**,
  with the discriminating parameter values (privacy-scrubbed — see §6). This is
  not "here's a prose repro"; it is a regression journey with a known-good
  prefix and a known-bad step, ready to run in CI. Binding is by property
  predicate, per §3.3 — an ordinal-bound extraction would produce false
  failures.

- **Matched nothing.** An unknown trajectory. Two things at once: a candidate
  new journey, and — usually — a corpus gap, because unmatched interactions
  cluster in T3/orphan territory. It feeds the same authoring queue the
  coverage loop already maintains.

### 4.2 Corpus → artifacts — Stage 2

Reuse the WebMCP compiler's architecture verbatim: `journeys + corpus → resolve
everything → IR → emit`. Only the emitters differ.

- **`--emit playwright`** — a spec file per journey. The playwright-bdd insight
  (keep the real runner; generate into it) without the step-definition layer,
  because the bindings *are* the step definitions and they're declarative.
- **`--emit webmcp`** — the existing generator, with journeys sliced at
  `navigate` boundaries into per-view tools gated by `require_view:`. The
  slicing rule is already implemented; journeys become a second front end to it.
- **`--emit steps`** — a compact, pre-resolved step list an agent executes with
  the `sightmap-browser` tools: the "pre-scripted sequences of browser
  interactions an agent can piece together to save tokens and increase
  reliability" from the original conversation. A journey becomes a macro the
  agent calls instead of re-deriving from a snapshot every time.
- **`--emit feature`** — a `.feature` file, derived per §3.2, **if** the
  round-trip proves wanted.

`generate --check` gates drift on all of them, as it already does for WebMCP
bundles.

### 4.3 SDLC triggers

| Trigger | What runs | Stage |
|---|---|---|
| **Per-PR** | Changed files → `dependencies:` globs → stale views/components → re-curate *those* → regenerate their bound journeys → run *only those* | 4 — but the index exists today |
| **Nightly cron** | Full re-curation sweep, full journey suite, drift report | 2 |
| **On Subtext session review** | §4.1 — file a corpus gap, a candidate journey, or a regression journey | 3, needs the enricher |
| **On corpus merge** | Regenerate every artifact; fail on drift | 2 |

### 4.4 Journey coverage — a behavioral tier ladder

Today's coverage is **spatial**: of the interactive nodes on this page, how many
resolve to a named component (T1/T2/T3)? The natural sibling is **behavioral**:
of the trajectories users actually perform, how many are described by a journey?

- **J1 — described.** The trajectory matches a known journey end to end.
- **J2 — partially described.** It matches a prefix, then diverges (the honest
  majority: users abandon, back out, retry).
- **J3 — undescribed.** No journey matches. The behavioral analogue of an
  orphan, and the authoring queue.

"J3 = 0 against last week's production traffic" is a much stronger done signal
than any line-coverage number. It is also **Stage 3 and enricher-dependent** —
computing it requires the same resolved stream §2.2 describes, so it is not
part of the near-term open-source story.

---

## 5. Why this is the right open-source wedge

Sightmap's OSS position today is "an agent-readable map of your app." Good, but
narrow: its value is legible mostly to teams already running coding agents
against a web app.

The wedge to sell is **§2.1 plus §4.2 — heal the map, and compile it into
tests.** Both are Sightmap-only, both are near-term, and together they're a
complete story that never needs to mention session replay:

> Your E2E tests break because selectors live in your tests. Move them into a
> map that an agent maintains against your running app, write your journeys
> against that map, and generate the tests. When the UI changes, the map heals
> once, in a PR you review, and every test that touched it heals with it. The
> journeys are YAML in your repo — you can leave whenever you want.

The production loop (§4.1) is the larger prize and the better demo, but it is
Stage 3 and it carries the §2.2 dependency. Leading with it would put a
capability that needs Subtext at the center of an open-source pitch, and
teams would discover the gap after adopting. Lead with heal-and-compile; let
the loop be the thing that gets *unusually* good if you also run an enricher.

What genuinely isn't open today, and is worth claiming:

- Gherkin is prose-only and its glue layer is per-repo, per-language, hand-written.
- Playwright's test plans are Markdown, per-tool, unversioned, unanchored.
- Every AI-testing vendor (Meticulous, Testim, Momentic, QA Wolf, Autify) has a
  proprietary internal flow representation you cannot export, diff, or leave with.

**There is no open, portable, executor-neutral, machine-checkable format for a
verified user journey.** That is a real hole, it sits directly on top of what
Sightmap already builds, and filling it pulls in contributors from the
Playwright and Cucumber communities rather than only from the AI-agent one.

---

## 6. Honest problems

1. **Sequence matching is fuzzy, and pretending otherwise will sink it.** Real
   users interleave, retry, and abandon. Step equality is the wrong primitive.
   The pragmatic v1 is **ordered-subsequence matching with a gap budget**, and
   J2 ("matched a prefix") must be first-class, not a failure mode.
2. **The backward read has a dependency (§2.2).** Stating it late — or letting
   a diagram imply otherwise — would be the single most damaging thing this
   proposal could do to its own credibility.
3. **Ordinal binding produces false failures (§3.3).** Extracted journeys must
   bind by property predicate; `#N` needs a lint rule, not just a convention.
4. **Assertions can re-import the glue layer (§3.3),** and SEP-0010's removal
   of `transform` leaves a real gap for value-shaped assertions. Signals are
   the intended answer; confirm that before promising typed comparisons.
5. **Privacy is first-order, not a footnote.** Production trajectories carry PII
   in the exact places journey params live. Extraction must run **behind**
   Subtext's privacy rules, bindings must be structural by default, and captured
   `example:` values must be opt-in and scrubbed. Getting this wrong once would
   be worse than not shipping.
6. **Pruning by traffic deletes rare-but-critical paths (§4.1).** Tags outrank
   frequency; frequency raises questions, never deletions.
7. **Journeys are a new maintenance surface.** Fifty that nobody curates is a
   worse artifact than five that are current — and #6 means the obvious
   automatic remedy is the dangerous one.
8. **`signals:` (SEP-0007) is not merged.** `expect: {signal: …}` depends on it,
   and §3.3 makes signals the preferred answer for value assertions, which
   *deepens* the dependency rather than softening it. Journeys can ship on
   components, requests, views and `exists:` alone; state the dependency.
9. **Scope discipline.** One SEP, one decision: the entity. Emitters, the
   session matcher, and journey coverage are follow-on work.

---

## 7. A staged plan

**Stage 0 — `sightmap journey record` / `journey run` (no spec change).**
Drive the browser with the existing CLI; emit a journey YAML in the §3.1 shape
(WebMCP step vocabulary + one `prose:` string); run it back with the existing
browser primitives. Ships as reserved tooling — the spec already has a
`Reserved tooling fields` escape hatch for exactly this. *Purpose: find out
whether the shape survives contact with three real apps before anyone writes an
SEP.* Do **not** start from a Gherkin-keyed shape (§3.2).

**Stage 1 — the SEP: the `journeys:` entity.**
One SEP, one decision: the entity, its steps, its bindings, its resolution
rules, and why `journeys:` and not `macros:`. *Don't announce a number when
floating the idea* — the process claims a number at draft-PR time, and
pre-claiming in a Discussion is exactly how two proposals collide. `0011` is
free today (0001–0010 on `main`, 0008 claimed by open PR #166); it may not be
when the PR opens.

**Stage 2 — emitters.**
Generalize the WebMCP compiler's `compile → IR → emit` spine to a second front
end. `--emit playwright` and `--emit steps` first. `--emit feature` only once
someone asks for the round-trip.

**Stage 3 — the session matcher (enricher-dependent).**
Align an enriched signal stream against corpus journeys; emit
`journey.matched` / `journey.failed_at_step` / `journey.unmatched`. Frame it as
"`signals:` with sequence" — SEP-0007 classifies a point, this classifies a
path. **Before committing:** spike the §2.2 question of whether the offline
matcher can run against a non-Subtext recording. The answer decides whether
this is an open capability or a commercial one, and that changes how Stage 1
should be pitched.

**Stage 4 — coverage and CI.**
`sightmap journey coverage` (J1/J2/J3 against a session set),
`dependencies:`-driven journey selection per PR, drift gates.

---

## 8. What to do next

1. **Float it.** A Discussion under Ideas, per the SEP process — this note is
   the float material, no SEP number attached.
2. **Stage 0, against three apps.** The docs site's own `.sightmap/`, the IKEA
   atlas corpus (already vendored for the WebMCP examples), and one real
   customer journey. Three journeys each, in the §3.1 shape. If it holds, write
   the SEP; if it doesn't, this note cost a week.
3. **Spike the §2.2 question** in parallel — can `match.MatchTree` align a
   journey against a recording that Subtext didn't produce? It's a day of work
   and it determines the honest shape of the pitch.
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
