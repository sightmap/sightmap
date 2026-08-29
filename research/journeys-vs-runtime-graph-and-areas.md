# Journeys, the runtime graph, and app areas: how they fit

> **Status:** research note, pre-SEP. Not normative. See [`README.md`](README.md).
> **Date:** 2026-08-29
> **Reads:** Clint's [Sightmap Runtime Graph] and [App Areas in the Sightmap] against
> [`journeys-and-agentic-verification.md`](journeys-and-agentic-verification.md).

---

## The short version

Three proposals are extending the same corpus with a verb layer. They are not
competing, but they are also not obviously complementary until you name what
each one is a proposition *about*:

| | Question it answers | Unit | Derivation |
|---|---|---|---|
| **Areas** (Clint) | *Where am I?* | A bounded surface | Declared membership by ref |
| **Runtime graph** (Clint) | *How do I get from A to B?* | An edge on a component | Derived on demand by BFS |
| **Journeys** (this branch) | *Which path has to keep working?* | A named, pinned path | Curated, with assertions |

The sharpest distinction: **the runtime graph computes paths; a journey commits
to one.** A graph has no opinion about which of its paths is load-bearing.
Every path is equally derivable and none is privileged, which is exactly right
for an agent trying to reach a goal and useless for a CI job trying to decide
what broke. A journey is the corpus saying *this specific path is the one users
take, and here is what must be true at each step*.

An area and a journey are also different shapes on the same surface: an area is
a region, a journey is a line through it. Clint already has the degenerate case
— `expect.path: [LoginPage, VerifyDevicePage]` is a journey with no step
bindings and no interactions, just an ordered view subsequence.

So the layering is clean, and it should be stated in that order: **areas bound
the surface, edges make movement derivable, journeys pin the paths that matter.**

---

## What the prior art changes in the journeys proposal

Six things, in descending order of how much they matter.

### 1. The enricher blocker is real, but I named the wrong one

The journeys note (§2.2) said the backward read depends on an enricher and
proposed a one-day spike to find out whether one could exist outside Subtext.
That framing is wrong on both counts.

Enrichment already exists. Authored component and request tags are unioned onto
`Signal.Tags` in the pipeline today, and the runtime-graph doc specs a
schema-aware `live-*` layer as "a stateless match pass over the loaded
sightmap, sitting between CDP output and the tool response." That is the
enricher, it is not proprietary magic, and it degrades gracefully on partial
coverage.

The actual blockers are narrower and worse, and Clint names both:

- **View tags don't reach the signal stream.** Enrichment consults components
  and requests only, and matches route-blind. A journey step keyed on `view:`
  cannot be matched backward until that changes.
- **There is no org-level persisted corpus.** A corpus exists per-session, via a
  single-use MCP upload. Watching a population needs the corpus to persist
  server-side.

The second one is not a spike. It is infrastructure, it gates areas and
journeys equally, and it should be sequenced as shared work rather than
discovered separately by each proposal.

### 2. Don't mint a second `expect:` dialect

The areas doc already defines one:

```yaml
expect:
  path: [LoginPage, VerifyDevicePage]     # ordered subsequence, detours allowed
  requests:
    Login: { status: "200" }              # SEP-0007 filter: dialect, verbatim
  never: [UncaughtLoginError]
```

`path` is an ordered view subsequence that tolerates detours. That is precisely
the "ordered-subsequence matching with a gap budget" the journeys note proposed
as its own pragmatic v1, arrived at independently and already scoped against a
real corpus. `never:` is what the journeys sketch called `not_expect:`.

Journeys should adopt this dialect unchanged and extend it only where a journey
genuinely needs more than an area does: per-step assertions rather than
per-area, and component-level bindings between the view waypoints. Two
constraint syntaxes in one corpus will drift; Clint made that argument for
`requests:` reusing SEP-0007 and it applies again here.

### 3. Numeric comparison is now blocking three proposals

SEP-0007 deferred comparison operators. Areas needs them ("no more than two
password attempts", "under 8 seconds"). Journeys needs them too — the rev-1
`matches: '\d+ results'` mistake was this gap wearing a disguise, and the
"three options" in §3.3 of the journeys note were all workarounds for its
absence.

Clint's call was to decide once rather than let each SEP choose. That was right
with two proposals and is more urgent with three. It should be resolved before
either areas or journeys goes to SEP, because both will otherwise ship a
constraint language they'd want to change.

### 4. `--emit steps` is already built, and better than the sketch

The journeys note proposed compiling a journey into a pre-resolved step list an
agent executes without re-snapshotting. That is `plan.py`, it exists, it has
been validated by playback in a real Subtext-Local session, and the token math
is measured rather than asserted: roughly 12 round-trips collapsing to one plan
plus one verification snapshot.

More importantly the planner *synthesizes* the mechanical steps from `requires:`
clauses. A journey that spells out every fill / blur / select is restating what
the graph already derives. A journey should name waypoints and assertions and
let the planner fill in the mechanics — which makes journeys much smaller
artifacts than the §3.1 sketch implies, and removes most of the maintenance
objection in §6.

### 5. Coverage: adopt the funnel, don't build a parallel ladder

The journeys note proposed J1/J2/J3 by analogy to T1/T2/T3. The index doc has a
better-specified version already:

```
114 raw signals
 → 112 matched a sightmap entity          98%  (named)
 →  90 that entity is an area member      79%  (attributed)
```

Two numbers, both deterministic, both computable with no LLM, and the second can
never exceed the first. Journey coverage is a third rung on the same funnel
(*of the attributed activity, how much lies on a declared journey?*), not a
separate ladder with its own vocabulary. Drop J1/J2/J3.

### 6. Three proposals now collide on SEP-0008

The areas doc plans to draft SEP-0008. PR #166 already claims 0008 for
parameterized view routes. The journeys note originally pre-claimed 0011 and was
corrected to claim nothing. Someone has to renumber, and it reinforces the rule:
claim a number at draft-PR time, never in a doc.

---

## What survives unchanged

**Heal the map, not the test.** Nothing in the prior art touches this, and the
runtime graph strengthens it: the more of the app's behavior lives in the
corpus, the more heals at once when a component's selector changes in a reviewed
diff. Clint's dream loop is the same argument pointed at authoring cost rather
than test maintenance.

**Test generation.** The runtime graph is built for agents *doing*; nothing in
either doc covers CI *verifying*. `--emit playwright` remains journeys-only
territory, and it is the piece that makes this legible to an engineering team
that doesn't run agents.

**The production-to-regression loop.** The index doc's deviation escalation
finds the anomalous session. It has no answer for what you do next so it can't
happen again. Pinning it as a journey with a known-good prefix and a known-bad
step is that answer, and the two halves fit together without either being
redesigned.

**Executor neutrality.** The code-first doc positions WebMCP as the *actions*
layer and sightmap as the *context and topology* layer, adjacent rather than
competing. That agrees with the journeys note's argument that a journey must
stay executor-neutral and be sliced per target.

---

## Where they genuinely pull against each other

Two tensions worth surfacing rather than papering over.

**Completeness versus curation.** The runtime graph wants exhaustive edges — the
connectivity audit reports orphan views, sink views, and unreachable views, and
132 edges on the Fullstory corpus is the point. Journeys want a small curated
set, and the journeys note argues that fifty stale ones are worse than five
current ones. Both are right for their own layer, but a team told to "declare
every edge" and "keep the set small" in the same corpus needs to know which rule
applies where. State it: **edges aim for completeness, journeys aim for
significance.**

**The dream loop shrinks the journeys use case, in a good way.** If prose
memories reliably promote into `on:` / `requires:` / `emits:`, then most
navigation knowledge becomes graph structure and journeys are left with only the
paths that carry assertions and business significance. That is the right
outcome, and it argues for scoping journeys narrowly from the start rather than
letting them become a second way to describe navigation.

---

## What I'd propose

1. **Sequence the shared blocker first.** Org-level persisted corpus plus view
   tags reaching the signal stream. Areas and journeys both wait on it; neither
   should discover it independently.
2. **Resolve numeric comparison once,** as an amendment to SEP-0007 or a small
   SEP of its own, before areas or journeys drafts.
3. **Areas goes first.** It is the coarsest layer, it has the clearest
   consumer (session watching at population scale), its `expect:` dialect is the
   one the other proposals should inherit, and the coverage funnel it defines is
   useful immediately.
4. **Runtime graph goes to the open spec, or explicitly doesn't.** Twenty-six
   internal corpus files now carry `on:`, `shells:`, `requires:`, and `network:`
   — schema that isn't in the open spec, with a naming split (`network:` vs the
   spec's `requests:`) already flagged. Every month that runs, the internal
   corpus and the open spec diverge further. This is the decision with the
   shortest fuse and it isn't really a journeys question at all.
5. **Journeys goes last and goes narrow.** A journey is a pinned path through
   the graph, carrying the areas `expect:` dialect at step granularity, compiled
   by the existing planner, emitted to Playwright for CI. That is a much smaller
   proposal than the one on this branch, and a better one.

---

## References

- Clint, *Sightmap Runtime Graph* (Notion) — `on:` edges, `shells:`, `network:`/`logs:`/`errors:`/`events:`/`cookies:`, `emits:`, BFS planner, dream loop. Branch `clint/sightmap-graph-on`; prototypes under `potemkin/village/.sightmap/runtime/`.
- Clint, *App Areas in the Sightmap: Declared Surfaces for Session Watching* (Notion) — `areas:`, `expect:`, `expected_ux:`, the index resolution and the named/attributed coverage funnel.
- [`journeys-and-agentic-verification.md`](journeys-and-agentic-verification.md) — the journeys proposal this note reconciles against.
- [SEP-0007 signals](../spec/seps/0007-signals.md) — the `filter:` dialect all three build on.
