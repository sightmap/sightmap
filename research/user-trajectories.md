# User trajectories, BDD, and Sightmap

**Status:** research note, not a proposal and not a spec change.
**Date:** 2026-08-29
**Prompt:** James Friend (Clay) shared Gherkin / Playwright-BDD / agentic BDD
references in the context of using Subtext for production session review and
Sightmap to give those reviews semantic views, components, requests, and
properties. This note asks how those pieces fit, and where an open-source
extension of Sightmap could create more value for engineering teams.

---

## Executive summary

Sightmap currently maps **nouns**: views, components, requests, properties,
memory. James is asking about **verbs**: named user journeys that can be
reviewed, replayed, composed, and healed as the product changes.

That gap is already named in the spec. `spec/v1/schema.md` lists, as an
unresolved v1 open question:

> Macros — learned trajectories that replay and heal when the site changes
> (not yet in the spec)

James's three references are the right prior art for filling it, but they
should be inverted rather than copied:

| Classic BDD | What this stack can actually do |
|---|---|
| Imagine examples in a workshop | Extract examples from production sessions (Subtext) and from the live app (Sightmap coverage + WebMCP trajectory walks) |
| Write Gherkin, then implement | Start from a map of the app that already exists; formulate the *delta* in journey language |
| Hand-write step definitions that click CSS | Bind steps to Sightmap names; compile selectors at generate time |
| Heal a broken test by rewriting locators | Heal the **map**; every trajectory, WebMCP tool, and generated test regenerates |

The open-source wedge is **not** another test runner, and it is not
"regenerate the app from BDD specs." It is a checked-in **trajectory layer**
on top of `.sightmap/` — user-goal sequences that address the product by
semantic name, compile into Playwright / browser sequences / WebMCP tools, and
stay valid because they never stored the fragile locator.

Subtext is the optional Discovery source: agentic session review distills a
real user path into a candidate trajectory. Sightmap is the durable
addressing layer that makes that trajectory reusable for testing. WebMCP is
already a first compiler of user-goal flows; it is the closest thing we have
today to James's "user flow components."

**Recommended next move:** float this as a GitHub Discussion, then a small
prototype that compiles corpus-bound flows (the WebMCP `flow:` vocabulary,
extended to cross-page journeys) into Playwright tests — *before* an SEP.
An SEP is warranted once we know whether trajectories belong in the corpus
or next to it.

---

## 1. What James actually proposed

Three nested ideas, not one:

### 1. Pair Gherkin steps with Sightmap entities

Gherkin is a natural-language format for *examples of behavior*. A step such
as `When the user adds "BILLY" to the cart` is deliberately UI-agnostic.
Cucumber made those steps executable by matching the trailing text against
**step definitions** — reusable chunks of scripting.

James's Sightmap version: a format that pairs each step with the **views,
components, and interactions that implement it**. The natural language stays
stable; the addressing lives in the map.

That is a clean split Cucumber never quite achieved. Classic step defs
usually leak UI (`page.getByRole('button', { name: 'Add to bag' })`). A
Sightmap-bound step would say `click: AddToCartButton` on `view:
ProductDetail` and let the corpus supply the selector.

### 2. A database of user flows, compiled into scripted sequences

[Playwright-BDD](https://vitalets.github.io/playwright-bdd/) generates native
Playwright tests from `.feature` files. James's analogue: maintain a
database of user flows, generate pre-scripted browser sequences for common
**user-flow components**, and let an agent piece those sequences together
instead of exploring the DOM from scratch. Token savings and reliability are
the stated goals.

This is the same instinct as WebMCP tools: a tool is a user goal, not a
page, and the agent sequences tools rather than clicking. The difference is
audience. WebMCP tools are for in-page agents. Flow components would also
serve CI tests and coding agents that need a cheap, reliable "do checkout"
primitive.

### 3. Agentic BDD taken to the extreme — and then walked back

Cucumber's BDD loop is Discovery → Formulation → Automation. The
"galaxy-brain" reading is: keep a database of BDD specs for the entire app
and regenerate implementation as requirements change.

James already flags that as hard as an opaque abstraction. The workable
product-development flow he describes is the **inverted** one:

> start with specs extracted from the actual app and then describe some
> change on top of that, and agents can handle building and verifying the
> delta

That inversion is the product. Do not try to generate Clay (or any app) from
Gherkin. Do extract a living model of what the app *does*, then let agents
implement and verify a named change against it.

---

## 2. The references, briefly

### Gherkin ([cucumber.io/docs/gherkin/reference](https://cucumber.io/docs/gherkin/reference))

A structured example language, not a test framework.

- `Feature` / `Rule` / `Scenario` (alias `Example`)
- `Given` (precondition) / `When` (event) / `Then` (observable outcome)
- `Background` for shared setup; `Scenario Outline` + `Examples` for tables
- Keywords are ignored when matching step definitions — the text is the
  identity. Duplicate step text with different keywords is a collision, which
  forces a clearer domain language.

The important Cucumber rules for us:

- **Then steps assert observable output**, not buried database state. That
  matches Sightmap properties and request-body properties exactly.
- **Given should not talk about UI.** "I am logged in" not "I click Log in."
  Implementation details belong in the binding, not the scenario.
- Scenarios should be 3–5 steps. Long scenarios stop being specifications.

Gherkin is a *projection* we can emit. It should not be the source of truth
inside `.sightmap/`. YAML that names corpus entities is the source of truth;
`.feature` files are one compiler output (alongside Playwright and WebMCP).

### Playwright-BDD ([vitalets.github.io/playwright-bdd](https://vitalets.github.io/playwright-bdd/))

A two-phase compiler:

1. `bddgen` turns `.feature` files into Playwright `test()` files that call
   `Given` / `When` / `Then` fixtures.
2. Playwright runs them with the usual runner: auto-wait, traces, shards,
   fixtures.

The 2026 post
[Why I Prefer BDD over SDD for Agentic Development](https://vitalets.github.io/posts/bdd-agentic-workflow/)
is the agentic argument: a Gherkin diff is a better planning surface than a
markdown spec pack (OpenSpec, GitHub Spec Kit) because the same artifact
becomes the acceptance test. The author still writes step definitions by
hand, with Playwright locators inside them.

That is the maintenance trap. Playwright-BDD gives you executable specs;
it does not give you a durable addressing layer. When the button's
accessible name changes, the step def breaks. A healer (see below) patches
the test. A Sightmap corpus would patch the component selector once.

### Cucumber BDD ([cucumber.io/docs/bdd](https://cucumber.io/docs/bdd))

Three practices, which map onto this stack unusually well:

| Practice | Question | Classic activity | This stack |
|---|---|---|---|
| **Discovery** | What *could* it do? | Example-mapping workshops | Production sessions (Subtext) + live trajectory walks (`sightmap-webmcp` Phase 1) |
| **Formulation** | What *should* it do? | Write Gherkin | Author a trajectory bound to Sightmap names; optionally emit `.feature` |
| **Automation** | What *does* it do? | Step defs + CI | Compile to Playwright / WebMCP / `sightmap browser` sequences; heal via the map |

The BDD docs are explicit that documentation and tests are side-effects.
The goal is valuable working software. For an app that already exists in
production — Clay, or any Subtext customer — Discovery is the scarce
resource, and Subtext already does it.

### Adjacent landscape (not in James's list, but it is the competition)

**Playwright Test Agents** ([planner / generator / healer](https://playwright.dev/docs/test-agents))
explore the app, write a Markdown plan, generate tests, and patch failing
locators. There is no durable semantic map. Healing rewrites tests. Plans
live in `specs/` as prose, not as typed references to named UI.

**DOM-similarity self-heal** (Healenium, BrowserStack Automate, various
"AI locator" products) swaps a broken CSS selector for a lookalike. It is
opaque, can hide real regressions, and does not accumulate product
knowledge.

**Spec-driven development** (GitHub Spec Kit, OpenSpec) produces markdown
that is not executable against the live UI. Vitaliy's Playwright-BDD
post is specifically an argument *against* this for UI behavior.

**WebMCP** ([github.com/webmachinelearning/webmcp](https://github.com/webmachinelearning/webmcp))
lets a page register tools on `document.modelContext`. Sightmap's
`sightmap webmcp` command is already a compiler from corpus +
`webmcp.tools.yaml` into those tools. A tool is a user goal. A live `flow:`
cannot navigate mid-call because a tool call dies with the document.
Cross-page journeys are explicitly out of scope and must be split into
per-page tools the agent sequences.

That last constraint is why WebMCP is not itself the trajectory database.
It is one **compile target** of a trajectory.

---

## 3. What Sightmap is today

A `.sightmap/` corpus is a checked-in, spec-validated map of:

- **Views** — named screens, route globs, representative URLs
- **Components** — named DOM subtrees, verified selectors, nested children,
  extracted properties
- **Requests** — named API routes, with optional response-body properties
- **Memory** — runtime lore that source code does not state
- **Messages** (SEP-0006) — console/exception patterns
- **Signals** (SEP-0007, draft) — named classifications composed from
  existing entities (`ref:` + property filter), never redeclaring
  selectors

Coverage (T1 / T2 / T3) measures whether interactive nodes have semantic
ancestors. It is **map quality**, not journey coverage. A corpus can have
T3 = 0 and still not know that "guest checkout" is a thing users do.

Curation loops that already exist:

- **Authoring loop** (`sightmap-authoring`): snapshot → sel-probe → YAML →
  coverage. Human or agent judgment for names.
- **PR recuration** (`.github/sightmap/curate-playbook.md`): on a UI diff,
  re-snap the mapped routes, close coverage gaps, do not invent product
  behavior. This is the "every PR changeset" trigger James's conversation
  alluded to.
- **WebMCP authoring** (`sightmap-webmcp`): walk a user-goal trajectory
  with the browser CLI, record the component queries and network calls, pick
  `api` / `fetch` / `live` kind, compile, live-verify.

The WebMCP manifest is the closest existing artifact to a trajectory:

```yaml
- name: search_products
  view: SearchResults
  flow:
    - navigate: "/us/en/search/?q={query}"
    - read:
        summary: { component: SearchSummary, property: summary }
        products:
          for_each: ProductCard
          fields:
            price: { component: CardPrice, property: price_text }
```

Properties of that format that matter:

- Every `component:` / `property:` / `request:` / `view:` **resolves through
  the corpus** at compile time. Nothing is guessed. Unresolved references
  fail loudly.
- Component queries use the same DSL as `sightmap browser click`
  (`Row[label="X"] Buy`).
- Descriptions fold `memory:` hazards into the text the agent decides by.
- Regeneration is drift-checked (`generate --check`, corpus content hash in
  the banner). Improve the map, regenerate the tools.

What it is not:

- It is not multi-page.
- It is not Given / When / Then. Reads are structured returns, not
  assertions.
- It is not sourced from production sessions.
- It is not a first-class corpus collection. The spec does not know about
  it; `validate` would warn on a `macros:` key in `.sightmap/` YAML (unknown
  field, by design, so experiments can be stashed).

The spec's reserved tooling fields (`access`, `snapshots`) and the
loader's skipped `review/` directory (punch-list YAML, not corpus) show a
pattern: **keep experimental or workflow artifacts next to the corpus
without pretending they are matching semantics.** Trajectories could start
the same way.

---

## 4. What Subtext is today

Subtext is agentic session review: capture every production session, let the
coding agent the team already uses open the ones that matter. James is
quoted on the Subtext homepage for the Clay workflow (Sentry alert → real
session → user impact, repro steps, validated fix).

The review skill (`/subtext:subtext-review`) is a structured investigation:

1. List sessions
2. Open → event map + digest (and a sightmap upload URL if the project has
   `.sightmap/`)
3. Zoom to the interesting window
4. Snapshot (screenshot + component tree + boxes)
5. Explain, hypothesize, suggest next steps
6. Close and record feedback

Sightmap's job in that loop is **token-efficient addressing**. The agent
sees `AddToCartButton (src/components/AddToCart.tsx)` instead of
`div.sc-4f8a9b > button:nth-child(2)`. Session review works without a corpus;
it is faster and more accurate with one.

Subtext Verify (companion plugin) is the live-browser / proof-docs side:
drive a hosted browser, capture before/after of a code change. That is
Automation's "does the delta work," not Discovery.

**The missing write-back.** A good session review already produces, in prose,
something that looks like a Gherkin scenario: given this user was on
ProductDetail, when they clicked AddToCartButton, then the bag count did
not update and CheckoutPayment returned `status: declined`. That prose is
currently ephemeral — a chat transcript, a ticket comment. It does not
become a checked-in trajectory, does not enrich the corpus, and does not
enter CI.

That write-back is the Subtext → Sightmap trigger the design discussions
have been circling.

---

## 5. How the pieces fit

```
                    production sessions
                    (Subtext capture)
                           │
                           ▼
                 agentic session review
              (Discovery: what users did)
                           │
                           │  distill + human/agent accept
                           ▼
              ┌────────────────────────────┐
              │  Trajectory (the new layer) │
              │  named user-goal sequences  │
              │  bound to Sightmap names     │
              └──────────────┬─────────────┘
                             │
           compile ──────────┼──────────────── compile
              │              │                     │
              ▼              ▼                     ▼
        Playwright      WebMCP tools      agent "flow
        tests /         (in-page,         components"
        Gherkin         single-doc)       (browser CLI
        projection                         sequences)
                             │
                             ▼
              .sightmap/ corpus (nouns)
              views · components · requests
              properties · memory · signals
                             │
                             ▼
                    selectors / routes
                    (the fragile layer)
                             ▲
                             │
              recuration on PR / cron / coverage fail
              (heal the map, not the tests)
```

Four layers, one source of truth for addressing:

1. **Intent** — a Feature / user story / ticket. Human language. Optional
   Gherkin projection.
2. **Trajectory** — ordered steps: view, component query, interaction or
   observation, expected property/request/signal. This is James's "database
   of user flows."
3. **Corpus** — Sightmap nouns. The only place selectors live.
4. **Compile targets** — Playwright, WebMCP, browser sequences, maybe a
   Gherkin file for humans who want Cucumber in the IDE.

Self-healing falls out of layer 3. If `AddToCartButton`'s selector changes
in a redesign, recuration updates the YAML. Every compiler output that
referenced the name is regenerated. A test that still fails after a
successful recuration is a **behavior change**, not a locator flake — which
is the failure you actually want CI to scream about.

Playwright's healer and DOM-similarity products collapse layers 2–4 into
a locator patch. That is cheaper in the moment and worse over a year,
because no product knowledge accumulates.

---

## 6. The inverted BDD loop (the product)

Classic BDD assumes the software does not exist yet. Clay's (and every
Subtext customer's) software does. The valuable loop is:

1. **Extract** the current behavior from the running app.
   - Sightmap: what can be named and addressed.
   - Subtext: which paths real users take, and which of those break.
   - WebMCP walks: which user goals are worth a tool.
2. **Formulate** a small set of trajectories that cover the paths that
   matter — happy paths, known failure modes (`signals:`), the ones that
   showed up in session review.
3. **Describe a delta** in the same language ("guest checkout should also
   work when the cart has a buy-back item").
4. **Agent implements the delta** against the map (source links on
   components) and **verifies** by replaying the affected trajectories plus
   the new one.
5. **Recurate** the map on the PR that landed the delta, so the next agent
   does not rediscover the new button.

This is James's "specs extracted from the actual app, then a change on
top." It is also why the galaxy-brain "regenerate the app from Gherkin" is
the wrong north star. The map is a *description of the running system*, not a
generative spec. Agents still need engineering judgment for data models,
architecture, and migrations — the same caveat Vitaliy makes in the
Playwright-BDD post.

---

## 7. SDLC insertion points

These are the triggers already in the design conversation, made concrete.

### A. Every PR that touches UI source (map maintenance)

Already sketched in the curate playbook. Scope: `.sightmap/` only, live
verify selectors, coverage must not drop on T1/T2.

**Add:** after recuration, **re-resolve every trajectory** (and every
WebMCP manifest) against the updated corpus. A renamed component or a
zero-match selector is a compile error on the PR, not a red Playwright
job two days later.

This is cheap and should ship as tooling before any spec change. It is
`sightmap webmcp validate` generalized to a trajectories file.

### B. Cron / nightly (drift against production or staging)

Coverage against representative view URLs. T3 regressions mean the map
rotted. Trajectory replay against staging means a user goal broke.

Nightly replay is optional and environment-dependent (auth, data).
Nightly **resolution** (do all names still match something?) is
environment-light and should be the default.

### C. Subtext session review (Discovery → candidate trajectory)

When a review concludes "this is a real user path we should keep
covering" — a converted checkout, a support-reproduced bug, a funnel
drop-off with a clear happy-path twin — the agent opens a PR that:

- adds or tightens corpus entities if the path walked unnamed components
  (T3 along the session)
- proposes a trajectory YAML (and optionally a Gherkin projection) with
  provenance: session id, timestamp, reviewer
- does **not** silently add it to CI until a human accepts

This is the only trigger that creates *new* journeys rather than
maintaining existing ones. It is also the one that makes Subtext more than
a debugger: it becomes a coverage-of-reality engine for the OSS map.

Guardrails:

- Privacy: trajectories must not embed PII. Bind to component *names* and
  property *shapes* (`quantity: 1`), not captured text from a real user.
- Dedup: many sessions are the same trajectory with different data.
  Cluster by view sequence + component-interaction signature, not by
  session id.
- Human accept: same as curator-vs-consumer. Agents draft; humans name.

### D. Agentic test execution (compose, don't explore)

Once a handful of trajectories exist, a coding agent working on a
checkout bug should **call `replay: GuestCheckout`** (or the equivalent
WebMCP tool sequence) rather than re-derive the click path from the DOM.
That is James's token/reliability argument, and it is the same reason
WebMCP exists for in-page agents.

Playwright Test Agents' planner would, in this world, plan in terms of
named trajectories plus a delta, not in terms of "click the textbox
named What needs to be done?"

---

## 8. What a trajectory might look like (illustrative, not a spec)

This is a sketch for discussion, not a schema. It deliberately looks like
WebMCP `flow:` plus Given/When/Then and multi-page permission.

```yaml
# .sightmap/trajectories/guest-checkout.yaml
# NOT valid corpus today. Would live next to the corpus, like webmcp.tools.yaml,
# until an SEP promotes it.

name: GuestCheckout
description: Signed-out user buys one in-stock product through checkout.
provenance:
  kind: session          # authored | session | synthetic
  session_id: "…"        # optional, Subtext
tags: [commerce, p0]

params:
  - { name: product_slug, type: string, required: true }

background:
  - goto: ProductDetail
    url: "/us/en/p/{product_slug}/"

steps:
  - when:
      click: AddToCartButton
      require_view: ProductDetail
    then:
      - property: { component: BagCount, name: count, changed: true }
      # or: signal: cart.item.added   (SEP-0007, if accepted)

  - when:
      click: MiniCartCheckout
    then:
      - view: Checkout

  - when:
      fill: { target: GuestEmail, value: "{synthetic_email}" }
      click: PlaceOrder
    then:
      - view: OrderConfirmation
      - request: CheckoutPayment
        # assertion against a declared request property, not a raw selector
```

A Gherkin projection of the same object:

```gherkin
Feature: Guest checkout
  Scenario: Signed-out user buys one in-stock product
    Given I am on a product page
    When I add the product to the cart
    Then the bag count increases
    When I check out from the mini-cart
    Then I am on checkout
    When I place the order as a guest
    Then I see the order confirmation
```

The Gherkin is for humans and for Playwright-BDD interop. The YAML is
what compilers consume. Step definitions are *generated* from the corpus,
not handwritten.

Reusable "user flow components" are just trajectories that other
trajectories can `ref:`, the same way SEP-0002 `$ref`s a component. Login,
search, add-to-cart, and pay are the obvious library. An agent composing
them is Cucumber's step-def reuse without the regex matching.

---

## 9. The open-source wedge

Sightmap's current OSS story: give every agent a shared, versioned map of
the running UI. That is real, and it is not enough for an engineering team
that already has Playwright and is being asked to "add AI testing."

What those teams actually pay for (in time) is **test rot**. Selectors
break, healers hide bugs, Gherkin step defs drift from the product, and
nobody trusts CI. The map-plus-trajectories design attacks that directly:

| Offer | Why it is OSS-native | Why it is not "just Playwright" |
|---|---|---|
| Checked-in `.sightmap/` | YAML + schema + CLI already ship | Playwright locators are not a shared product model |
| Trajectories that name the map | Same directory, same review process as code | Tests become a projection, not the source of truth |
| Compile to the runner they have | Emit Playwright (and optionally Gherkin) | We do not need them to adopt our runner |
| Heal by recuration | PR bot they can run in GitHub Actions | Healing knowledge lands in git, not in a SaaS locator DB |
| Session-derived candidates | Subtext is optional | OSS path: author trajectories by walking the app with `sightmap browser`, same as WebMCP |

**Subtext remains the hosted Discovery engine.** That is a clean commercial
split: anyone can map and test; teams that capture production get better
trajectory candidates and better signal on which journeys matter. The OSS
project must not require Subtext to be useful — same principle as the
corpus itself.

**Do not build:**

- A new test runner
- An LLM that rewrites Playwright locators (Playwright already has a healer)
- A system that generates application code from Gherkin
- Gherkin as a normative Sightmap collection (keep it as a projection)

**Do build, in this order:**

1. **A trajectories file format next to the corpus** (like
   `webmcp.tools.yaml`), validated by resolving every name against
   `.sightmap/`. No SEP required if it is tooling, not matching semantics.
   Unknown-field warnings already let people stash `macros:` in corpus
   YAML; better to keep it in a sibling file so `validate` stays strict.
2. **A compiler to Playwright** (and a Gherkin dump for people who want
   Playwright-BDD). Prove James's "generate pre-scripted sequences" with
   one worked example (IKEA `add_to_cart` + search is sitting in
   `webmcp/examples/`).
3. **CI resolution check** on PRs that touch `.sightmap/` or UI source —
   generalize `webmcp validate`.
4. **A skill** that walks a Subtext review and drafts a trajectory PR,
   including any missing components the session revealed (T3 along the
   path). This is the Subtext trigger; it can live in the Subtext plugin
   and emit OSS-shaped YAML.
5. **SEP** only when we know whether trajectories belong *in* the spec
   (peers to `views` / `components` / `signals`) or stay as reserved
   tooling like `snapshots`. Default bias: **sibling, not spec**, until a
   second consumer needs to interpret them. WebMCP already taught us that a
   compiler can be powerful without being normative.

The naming fork: the spec says "macros"; James says "user flows" /
"user-flow components"; WebMCP says "tools" / "trajectories." For OSS
messaging, **trajectory** is the most accurate (a path through named
views). "Macro" sounds like a keybinding. "Flow" collides with WebMCP's
`flow:` key. Pick one in the Discussion and stick to it.

---

## 10. Risks and non-goals

**Trajectories that encode a flaky UI.** If a Then asserts a client-rendered
region via `mode: fetch`, it will fail the way WebMCP fetch-tools already
fail. The WebMCP skill's kind-selection table (api / fetch / live) has to
be inherited.

**Over-coverage.** Recording every session as a trajectory is how you
reproduce Cucumber's "10,000 scenarios nobody reads." Accept only
clustered, named, P0/P1 paths. Session review is a *proposal* generator.

**Healing that hides product bugs.** Recuration that "fixes" a selector
when the feature was actually removed is the DOM-healer failure mode.
Guard: if a component's selector goes to zero matches and `source:` still
exists, that is a failing test, not a heal. If `source:` is gone in the
same PR, the trajectory should be updated or dropped in that PR, as a
deliberate diff.

**Auth and data.** Guest-checkout is easy. "User with a saved card and a
partial return" is not. Trajectories need the same `access:` honesty the
curate playbook already uses on views (`open` / `blocked` /
`needs-data`). Do not pretend every journey is replayable in CI.

**WebMCP's document-lifetime constraint** remains. A trajectory that
crosses pages cannot be *one* WebMCP tool. Compilers must split. That is a
feature: the agent-facing tools stay small; the trajectory is the thing
that knows the sequence.

---

## 11. Suggested Discussion prompt (for GitHub)

> Sightmap maps nouns (views, components, requests). We do not yet have a
> first-class representation of user journeys. The spec's open question
> calls these "macros"; WebMCP already compiles single-page user-goal
> flows; Subtext session review already distills real paths in prose.
>
> Proposal: keep a sibling `trajectories/` (or `webmcp.tools.yaml`-like)
> file that binds Given/When/Then-shaped steps to corpus names, compile
> that into Playwright (and Gherkin, optionally), and heal by recuration
> of the map rather than by rewriting tests. Subtext reviews become a
> source of *candidate* trajectories, not a required runtime.
>
> Questions we want input on:
> 1. Sibling tooling vs. a spec collection (`trajectories:` / `macros:`)?
> 2. Is WebMCP `flow:` the right vocabulary to extend, or do assertions
>    need a different shape?
> 3. What is the smallest compile target worth shipping — Playwright,
>    `sightmap browser` sequences, or both?

---

## References

- [Gherkin reference](https://cucumber.io/docs/gherkin/reference)
- [Behaviour-Driven Development](https://cucumber.io/docs/bdd) (Discovery,
  Formulation, Automation)
- [Playwright-BDD](https://vitalets.github.io/playwright-bdd/)
- [Why I Prefer BDD over SDD for Agentic Development](https://vitalets.github.io/posts/bdd-agentic-workflow/)
  (Vitaliy Potapov, 2026-07-10)
- [Playwright Test Agents](https://playwright.dev/docs/test-agents)
- [WebMCP proposal](https://github.com/webmachinelearning/webmcp)
- Sightmap spec open questions: `spec/v1/schema.md` ("Macros — learned
  trajectories…")
- WebMCP compiler: `webmcp/README.md`, `skills/sightmap-webmcp/SKILL.md`,
  `docs/cli/webmcp.mdx`
- Subtext session review:
  [docs](https://subtext.fullstory.com/docs/session-review/overview)
- SEP-0007 (draft): Signals — named classifications from existing entities
- Curate playbook: `.github/sightmap/curate-playbook.md`
