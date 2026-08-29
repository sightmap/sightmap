# Flows: verified user trajectories as a first-class Sightmap entity

> **Status:** research note, pre-SEP. Not normative. See [`README.md`](README.md).
> **Date:** 2026-08-29
> **Prompted by:** a conversation with James (Clay) on Gherkin, playwright-bdd,
> and agentic BDD, in the context of Subtext session review + Sightmap.

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
`flows:` entity in the corpus, where each step carries Gherkin-shaped prose
**and** a binding to named corpus entities, is that form. It is simultaneously
a script an executor can run and a predicate a session stream can be matched
against, which is the property that makes the whole loop close.

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
  flow step should bind to.

And the spec already flags the gap, in its own words:

> **Open questions** … Macros — learned trajectories that replay and heal when
> the site changes (not yet in the spec)
> — [`spec/v1/schema.md`](../spec/v1/schema.md)

### 1.2 The WebMCP branches (in review, PRs #281 / #292 / #293 / #294)

This is the most important prior art in the repo, and it is *already* a flow
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
that the manifest already had to model it is evidence that a flow
representation must be **executor-neutral** and sliced per target.

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
| **[Meticulous](https://www.meticulous.ai/how-it-works)** | Records real sessions, replays events against a build, diffs visual snapshots, mocks the backend, curates a session set for code coverage. Genuinely closes the prod→test loop. | Tests are opaque event streams, not readable specs. You cannot *edit the intent*, only re-record. Proprietary, hosted, and the artifact isn't yours. |
| **Stagehand / Midscene / Testim-class healers** | Runtime resilience is real and users like it. | Healing is per-test, unreviewable, and repeated N times for one UI change. The knowledge never accrues anywhere. |
| **[Spec Kit](https://github.com/github/spec-kit) / Kiro (EARS)** | The galaxy-brain framing, productized: spec as unit of work, code as regenerable output, requirement→task traceability. | Specs are written **ahead of** the code and drift the moment they're merged. Nothing derives them from observed behavior. |

**The pattern across every row:** the industry has converged on "a
human-readable flow spec, plus an agent, plus a browser." Everyone has the
prose. Nobody has a **verified, versioned, portable binding from prose to app
structure** — because producing one requires exactly the curated, coverage-
scored, source-linked map that Sightmap already builds.

---

## 2. The central claim

> ### Heal the map, not the test.

Every self-healing product on the market heals *the test*: a locator breaks,
an LLM looks at the DOM, guesses a new locator, patches that one test, at
runtime, without review. One UI change costs N heals across N tests, produces
N opaque decisions, and leaves nothing behind.

Sightmap inverts it. The selector lives in exactly one place — the corpus. A PR
touches `src/components/DatePicker.tsx`; `dependencies:` says which views and
components are now stale; the authoring agent re-curates *those* against the
live app with the existing coverage loop; the selector changes **once, in a
reviewed diff**. Every flow bound to that component regenerates. So do the
WebMCP tools. So does session enrichment for every replay from now on.

One heal, reviewed, diffable, amortized across every consumer. That is a
categorically different product than runtime locator-guessing, and it is only
available to someone who already maintains the map.

The corollary is the second claim:

> ### A spec derived from the running app is a spec you can't lie in.

Spec Kit and Kiro write specs ahead of the code; they drift on merge, and the
drift is silent. A Sightmap flow is *extracted from behavior* — recorded from a
real browser session or a real production trajectory, with every binding
resolved against the live DOM at curation time and re-checked by CI. The
"galaxy-brain" regenerate-the-app-from-specs idea fails as an opaque
abstraction, but it works fine as a **delta**: start from a spec you extracted
from the app as it is, describe the change on top, let agents build and verify
the difference. That is the only version of spec-driven development where the
starting spec is known to be true.

---

## 3. The proposal sketch: `flows:`

### 3.1 Shape

A flow is a named, parameterized sequence of steps. Each step carries Gherkin
prose **and** a binding to corpus entities. No step ever contains a selector.

```yaml
version: 1

flows:
  - name: SearchAndAddToCart
    description: Find a product by keyword and add it to the basket
    tags: [revenue, smoke]
    source: src/features/cart/            # optional, same meaning as elsewhere
    dependencies: ['src/features/search/**']
    memory:
      - The cart badge lags the request by ~300ms; assert on the request, not the badge
    params:
      - { name: query, type: string, example: "desk chair" }

    steps:
      - given: I am on the product search page
        view: SearchResults
        url: /us/en/search/?q={query}

      - when: I search for "{query}"
        act:
          - fill:  { component: SearchInput, value: "{query}" }
          - click: { component: SearchButton }
        await:
          request: SearchAPI               # corpus requests: entry
        then: I see a result summary and product cards
        expect:
          - { component: SearchSummary, property: summary, matches: '\d+ results' }
          - { exists: ProductCard }

      - when: I open the first result
        act:
          - click: { component: 'ProductCard#1' }   # compquery, already implemented
        then: I land on the product detail view
        expect:
          - { view: ProductDetail }

      - when: I add it to the cart
        view: ProductDetail
        act:
          - click: { component: AddToCartButton }
        await:
          request: AddToCart
        then: the item is in the basket
        expect:
          - { request: AddToCart, status: 200 }
          - { signal: cart.item_added }     # SEP-0007 signal
        not_expect:
          - { signal: checkout.payment.declined }
```

Design rules, each with a reason:

| Rule | Why |
|---|---|
| `given` / `when` / `then` carry **prose only**, and are round-trippable to a `.feature` file | A PM can read it; Cucumber tooling can consume it; but nothing *executes* by matching the prose |
| Bindings reference corpus entities **by name**, never by selector | This is the entire healing mechanism; it is also the WebMCP manifest's existing rule |
| Step reuse is by explicit `$ref`, not by regex-matching step text | Regex step-matching **is** the Cucumber glue layer. It is the part that rots. Sightmap already has `$ref` with defined resolution and collision semantics — reuse it |
| `expect:` composes existing entities: `component`+`property`, `request`+`status`, `signal`, `view`, `exists` | Nothing new to evaluate. SEP-0005 request properties, SEP-0007 signals, and SEP-0010 tree-closed component properties already do the work |
| `params:` with `example:` | Gherkin `Scenario Outline`/`Examples` maps onto this directly; the `example` is what a recorded flow captured, and what CI runs by default |
| Flows carry `tags:`, `memory:`, `dependencies:`, `source:` like every other entity | Consistency, and `dependencies:` is what makes per-PR test selection work |

### 3.2 The property that makes it worth doing

**The same YAML reads in two directions.**

*Forward (executor):* `click: {component: AddToCartButton}` compiles — via the
existing corpus resolution — to a selector chain a Playwright locator, a WebMCP
runtime action, or a `sightmap browser click` can execute.

*Backward (matcher):* the same line is a **predicate over an enriched signal
stream**. Subtext already resolves an interaction signal to the component it
targeted. "Did this session click AddToCartButton?" is a lookup, not an
inference. `await: {request: AddToCart}` matches a network signal resolved to
that request entity. `expect: {signal: cart.item_added}` matches a SEP-0007
classified signal.

No other flow format on the market can be read backwards, because no other
format's steps are anchored to entities that a session recorder independently
resolves. This is the moat, and it comes for free from work already shipped.

---

## 4. The loop this closes

```
                    ┌─────────────────────────────────────────────┐
                    │                                             │
   production        ▼                                            │
   sessions ──► Subtext review ──► enriched signal stream         │
                                          │                       │
                                   flow matcher                   │
                    ┌─────────────────────┼─────────────────────┐ │
                    ▼                     ▼                     ▼ │
            matched + passed      matched + failed@N      unmatched│
                    │                     │                     │ │
          frequency evidence      reproduction, anchored   NEW TRAJECTORY
          (which flows matter)    at a step boundary       (corpus gap +
                    │                     │                 candidate flow)
                    │                     ▼                     │ │
                    │            regression flow ───────────────┘ │
                    │            (params from the real session)   │
                    ▼                     │                       │
            ┌───────────────────────────────────────────┐         │
            │            .sightmap/ flows:              │─────────┘
            └───────────────────────────────────────────┘
                    │
     ┌──────────────┼───────────────┬──────────────────┐
     ▼              ▼               ▼                  ▼
  Playwright    WebMCP tools    agent step-lists   flow coverage
  spec files    (existing!)     (token-cheap        report
                                 pre-scripted
                                 sequences)
```

### 4.1 Production → corpus (the Subtext trigger)

A session's enriched signal stream is aligned against every known flow. Three
outcomes, each actionable:

- **Matched, succeeded.** Frequency evidence. "`SearchAndAddToCart` ran 4,102×
  this week; `AdminBulkExport` ran twice." That is the only honest input to
  *which tests are worth maintaining* — and it is exactly what Meticulous
  charges for, except here the output is a readable spec you own.
- **Matched, failed at step N.** A reproduction **anchored at a step boundary**,
  with the real parameter values (privacy-scrubbed — see §6). This is not
  "here's a prose repro"; it is a regression flow with a known-good prefix and
  a known-bad step, ready to run in CI. This is the Subtext-side trigger the
  design discussions were reaching for.
- **Matched nothing.** An unknown trajectory. Two things at once: a candidate
  new flow, and — usually — a corpus gap, because unmatched interactions
  cluster in T3/orphan territory. It feeds the same authoring queue the
  coverage loop already maintains.

### 4.2 Corpus → artifacts

Reuse the WebMCP compiler's architecture verbatim: `flows + corpus → resolve
everything → IR → emit`. Only the emitters differ.

- **`--emit playwright`** — a spec file per flow. The playwright-bdd insight
  (keep the real runner; generate into it) without the step-definition layer,
  because the bindings *are* the step definitions and they're declarative.
- **`--emit webmcp`** — the existing generator, with flows sliced at
  `navigate` boundaries into per-view tools gated by `require_view:`. The
  slicing rule is already implemented; flows just become a second front end to it.
- **`--emit steps`** — a compact, pre-resolved step list an agent executes with
  the `sightmap-browser` tools. This is James's "pre-scripted sequences of
  browser interactions for common user flow components which could be pieced
  together by an agent to save tokens and increase reliability." A flow becomes
  a **macro** the agent calls instead of re-deriving from a snapshot every time.
- **`--emit feature`** — a `.feature` file, for teams that already run Cucumber
  and for the PM-readable view. Round-trip is why the prose keys exist.

`generate --check` gates drift on all of them, as it already does for WebMCP
bundles.

### 4.3 SDLC triggers

| Trigger | What runs | Enabled by |
|---|---|---|
| **Per-PR** | Changed files → `dependencies:` globs → stale views/components → re-curate *those* → regenerate their bound flows → run *only those flows* | `dependencies:` is already a change-impact index; nothing consumes it yet. This is test-impact analysis and targeted re-curation from one declaration |
| **Nightly cron** | Full re-curation sweep, full flow suite, drift report | Existing coverage + `generate --check` |
| **On Subtext session review** | §4.1 — file a corpus gap, a candidate flow, or a regression flow | New; the highest-leverage piece |
| **On corpus merge** | Regenerate every artifact; fail on drift | Existing |

### 4.4 Flow coverage — a behavioral tier ladder

Today's coverage is **spatial**: of the interactive nodes on this page, how many
resolve to a named component (T1/T2/T3)? The natural sibling is **behavioral**:
of the trajectories users actually perform, how many are described by a flow?

A first cut, deliberately mirroring the existing tiers so the mental model
transfers:

- **F1 — described.** The trajectory matches a known flow end to end.
- **F2 — partially described.** It matches a known flow's prefix, then diverges
  (the honest majority: users abandon, back out, retry).
- **F3 — undescribed.** No flow matches. The behavioral analogue of an orphan,
  and the authoring queue.

"F3 = 0 against last week's production traffic" is a *much* stronger done signal
than any line-coverage number, and it is one Sightmap is uniquely positioned to
compute, because it already resolves both halves — the map and the stream.

---

## 5. Why this is the right open-source wedge

Sightmap's OSS position today is "an agent-readable map of your app." Good, but
narrow: its value is legible mostly to teams already running coding agents
against a web app.

`flows:` widens it to a category nobody owns. Consider what is *not* currently
open:

- Gherkin is prose-only and its glue layer is per-repo, per-language, hand-written.
- Playwright's test plans are Markdown, per-tool, unversioned, unanchored.
- Every AI-testing vendor (Meticulous, Testim, Momentic, QA Wolf, Autify) has a
  proprietary internal flow representation you cannot export, diff, or leave with.

**There is no open, portable, executor-neutral, machine-checkable format for a
verified user flow.** That is a real hole, it sits directly on top of what
Sightmap already builds, and filling it is the kind of thing that pulls in
contributors from the Playwright and Cucumber communities rather than only from
the AI-agent community.

The pitch to an engineering team is concrete and doesn't mention agents:

> Your E2E tests break because selectors live in your tests. Move them into a
> map that an agent maintains against your running app, write your flows in
> Gherkin bound to that map, and generate the tests. When the UI changes, the
> map heals once, in a PR you review, and every test that touched it heals with
> it. The flows are YAML in your repo — you can leave whenever you want.

And it makes Subtext strictly more valuable without making Sightmap depend on
it: sessions stop being summarized one at a time and start being **classified
against a known behavioral model of the app**.

---

## 6. Honest problems

None of these are fatal; all of them need an answer before an SEP.

1. **Sequence matching is fuzzy, and pretending otherwise will sink it.** Real
   users interleave, retry, open new tabs, and abandon. Step equality is the
   wrong primitive. The pragmatic v1 is **ordered-subsequence matching with a
   gap budget**: steps must occur in order, unrelated signals between them are
   allowed up to a threshold, and a match reports how many steps it got
   through. F2 ("matched a prefix") is not a failure mode, it is the normal
   case and it must be first-class.
2. **The step-text trap.** Gherkin's reusability comes from prose matching, and
   prose matching is precisely the layer that rots. The proposal deliberately
   makes prose non-load-bearing (documentation + round-trip only) and reuse
   explicit via `$ref`. This will feel wrong to anyone with Cucumber muscle
   memory and the SEP has to argue it directly.
3. **Executor neutrality vs. executor constraints.** WebMCP flows cannot
   navigate mid-flow; Playwright flows can; an agent step-list can do either.
   The flow entity has to stay neutral and let each emitter slice. The
   `require_view:` machinery shows this is tractable, but it means some flows
   are legal and some emitters will legitimately refuse them — the compiler
   must say so clearly rather than degrading silently.
4. **Privacy is a first-order concern, not a footnote.** Production
   trajectories carry PII in the exact places flow params live (search terms,
   emails, addresses). Extraction must run **behind** Subtext's existing
   privacy rules, bindings must be structural by default (component identity
   and property *presence*, not values), and captured `example:` values must be
   opt-in and scrubbed. Getting this wrong once would be worse than not
   shipping.
5. **Flow curation is a new maintenance surface.** Fifty flows that nobody
   prunes is a worse artifact than five that are current. The frequency
   evidence from §4.1 is the mitigation — flows that production stopped
   exercising should surface for deletion, the way `capture-prune` already
   handles stale captures.
6. **`signals:` (SEP-0007) is not merged.** `expect: {signal: ...}` depends on
   it. Flows can ship expecting only components, requests, and views, with
   signals folded in when #114 lands — but the dependency should be stated, not
   discovered.
7. **Scope discipline.** The SEP README is explicit: one SEP, one decision. The
   `flows:` entity is one SEP. Emitters, the session matcher, and flow coverage
   are follow-on work and must not ride along in the proposal.

---

## 7. A staged plan

Sequenced so each stage is independently useful and de-risks the next.

**Stage 0 — `sightmap flow record` / `flow run` (no spec change).**
Drive the browser with the existing CLI; emit a flow YAML from the trajectory;
run it back with the existing browser primitives. Ships as reserved tooling
(the spec already has a `Reserved tooling fields` escape hatch for exactly
this). *Purpose: find out whether the shape survives contact with three real
apps before anyone writes an SEP.* Cheapest possible falsification.

**Stage 1 — SEP-0011: the `flows:` entity.**
`0011` is the next free number (0001–0010 on `main`; 0008 claimed by open PR
\#166). One SEP, one decision: the entity, its steps, its bindings, and its
resolution rules. Conformance fixtures in the matching reserved range.

**Stage 2 — emitters.**
Generalize the WebMCP compiler's `compile → IR → emit` spine to a second front
end. Ship `--emit playwright` and `--emit steps` first; `--emit feature` is
cheap and buys the Cucumber crowd; `--emit webmcp` is mostly re-pointing the
existing generator.

**Stage 3 — the session matcher (Subtext side).**
Align an enriched signal stream against corpus flows; emit
`flow.matched` / `flow.failed_at_step` / `flow.unmatched`. Frame it as
"`signals:` with sequence" — SEP-0007 classifies a point, this classifies a
path — so it lands as an extension of an accepted idea rather than a new one.

**Stage 4 — coverage and CI.**
`sightmap flow coverage` (F1/F2/F3 against a session set), `dependencies:`-driven
flow selection per PR, drift gates. This is where the per-PR and cron triggers
from §4.3 actually become products.

---

## 8. What to do next

1. **Float it.** A Discussion under Ideas, per the SEP process — this note is
   the float material. James is the obvious first reader; he asked the question.
2. **Stage 0, against three apps.** The docs site's own `.sightmap/`, the IKEA
   atlas corpus (already vendored for the WebMCP examples), and one real Clay
   flow. Three flows each. If the shape holds, write the SEP; if it doesn't,
   this note cost a week.
3. **Land the WebMCP branches.** They are the compiler this depends on, and
   four of them are open right now (#281, #292, #293, #294).
4. **Answer the privacy question in writing before Stage 3.** Not after.

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
- In-repo: [`spec/v1/schema.md`](../spec/v1/schema.md) · [SEP-0007 signals](../spec/seps/0007-signals.md) · [coverage model](../go/docs/reference/coverage-model.md) · [outer loop](../go/docs/reference/outer-loop.md) · [architecture](../go/ARCHITECTURE.md)
