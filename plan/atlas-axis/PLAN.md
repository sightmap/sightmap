# Phase 6 — AXIS scorecards

Build order for [`SPEC.md`](SPEC.md): give every atlas entry a measured
before/after agent-experience score, published on sightmap.org and in the atlas's
machine surfaces.

Slots after P5 in the atlas [implementation plan](https://github.com/sightmap/atlas/blob/main/plan/IMPLEMENTATION.md).
The tasks marked **atlas** are written in that repo's phase style and should be
lifted to `plan/phases/phase-6-axis.md` there when this is accepted; they live
here only because this branch is in the monorepo.

**Read P6.0 as the whole of the near-term plan.** Everything after it is written
so the spike knows what it is de-risking, and every task from P6.1 on is
contingent on what the spike finds. We are, per Sean, extending AXIS past what it
was built for (SPEC §2.4) — the shape of P6.1–P6.7 should be expected to change.

## Working agreement

Same as the rest of the atlas plan: one task = one PR, referenced by task ID.
Acceptance criteria are verified with real command output, not asserted. Contracts
live in [`SPEC.md`](SPEC.md) and in atlas `docs/SPEC.md`; a contract change happens
in the same PR that needs it, loudly.

## Dependencies

| Task | Repo | Depends on |
|---|---|---|
| P6.0 spike | atlas | P1 (`atlas add`), P3 (seed entries) |
| P6.1 harness scaffold | atlas | P6.0 go |
| P6.2 scenarios + skill | atlas | P6.1 |
| P6.3 record + index | atlas | P6.2 |
| P6.4 scoring workflow | atlas | P6.3 |
| P6.5 gallery surface | sightmap/sightmap `web/` | P6.3 (schema), P4 (gallery) |
| P6.6 regression + freshness | atlas | P6.4, P5.1 |
| P6.7 launch content | both | P6.5 |

P6.5 can be built against a fixture `axis` block as soon as P6.3 fixes the schema;
it does not wait for real scores.

---

## P6.0 Feasibility spike — **the gate, and the current focus**

Throwaway branch, no merge, no schema, no automation. Everything downstream
assumes the lift is real, affordable, and mechanically possible; this is where we
find out.

### Targets: two sites, deliberately unlike each other

**`sightmap-org`** — 4 views, 28 components. Our own property, no third-party
exposure, fully deterministic. If the ladder does not separate here, it will not
separate anywhere, and we will have learned that cheaply.

**One complex seed** — the point of the second target is that sightmap.org is a
small marketing site, and a map's value should grow with surface complexity. The
seeds available are amazon, ebay, ikea, nike, vuori, uniqlo, airbnb, apple.

Take **airbnb** first: 5 views, 54 components, 16 requests, and a genuinely
multi-step task surface (search → results with a live map → listing detail) that
exercises `traverse` properly. If it turns us away, fall back in order to
**ikea**, **apple**, **ebay**. Nike and Amazon are the least likely to tolerate an
automated browser.

**Pre-flight before writing any scenario:** point the managed Chrome at each
candidate and record what happens — served normally, soft-blocked, CAPTCHA wall.
This costs an afternoon and is worth doing regardless of the spike's outcome,
because it is the same question the whole `live` tier depends on (SPEC §7). A
site that turns us away gets recorded and skipped, never worked around.

### Shape

Two scenarios per site (one `navigate`, one `extract`), three variants
(SPEC §2.2), three repetitions. 36 jobs. Real numbers on cost and wall-clock come
out of this; the estimate that motivated the schedule in P6.4 does not.

### What the spike has to answer

**Does the ladder separate?** Is `with-sightmap` − `with-skills` bigger than the
run-to-run spread, at n=3, on both sites? Is the gap wider on the complex site
than on sightmap.org, as the whole premise predicts? Which dimensions moved, and
did Goal Achievement carry it (SPEC §6.6)?

**Does the middle rung behave?** `with-skills` should land between the other two.
If it lands level with `with-sightmap`, the tooling is doing the work and the map
is not — the single most important thing this spike can tell us, and better
learned now than after five more phases.

**Is SPEC §2.4 right about Service?** Confirm that browser interactions classify
as Environment and the site is invisible to Service. If so, that is a real finding
to take back to Sean, and it shapes what §5 puts on the page.

**Do the mechanics hold?**
- Chrome installed once in `beforeAll`, symlinked per job across isolated `HOME`s.
- Does AXIS's `skills` field actually reach the agent, or is
  `sightmap skills install --target .claude/skills` needed instead?
- Can a coding agent in an AXIS workspace drive `sightmap browser` without
  hand-holding, and does the session survive the workspace teardown?
- Does a 12-minute limit fit a `traverse` task on a real site?

**What does it cost?** Tokens and wall-clock per job, extrapolated to a full
entry and to a full-atlas sweep.

### Deliverable

A written go/no-go with the raw `report.json` attached: the numbers, the mechanical
findings, the pre-flight results, the cost extrapolation, and — if no-go — what
would have to change. **A weak or noise-level lift is a legitimate outcome and
stops the phase.** So is "airbnb blocks us and ikea is flaky"; that would push the
whole design toward `self-hosted` and `replay` before anything else gets built.

### Optional precursor — Track A, if the harness fights us

If AXIS mechanics turn out to be the hard part rather than the measurement, run a
handful of Track A scenarios first (SPEC §2.5): score the sightmap CLI and skills
as a dev-tool experience, no browser and no live sites, structured exactly like
`netlify/context-and-tools`. It exercises config, variants, judging, and reports
against a target that cannot flake, and it produces a useful quality signal for
the skills either way. Not required; a shortcut if P6.0 stalls on plumbing.

---

## P6.1 Harness scaffold — atlas

- `axis.config.ts` at the atlas repo root per SPEC §3.5: pinned `@netlify/axis`,
  pinned canonical agent and model, `judging.agents` resolving to a different
  adapter, default `scoring_weights`, `concurrency: 2`, mandatory limits.
- `axis/helpers/variants.ts` — the three-rung ladder from SPEC §3.4, the one place
  the ladder is defined.
- `axis/bin/link-chrome.sh` + the `beforeAll` install, whatever shape P6.0 proved.
- `.gitignore`: `.axis/reports/`, `.axis/remotes/`. `.axis/baselines/` **is**
  committed.
- Acceptance: `npx axis run --scenario <fixture>` green from a clean checkout on a
  machine that has never installed Chrome, all three rungs completing.

## P6.2 Scenario contract + authoring — atlas

- `axis/README.md`: the archetype table (SPEC §3.1) and the prompt/rubric rules
  (SPEC §3.2) as contributor-facing guidance.
- `schema/axis-scenario.schema.json` (or a TS type guard, since scenarios are
  modules): validates archetype membership, the three required rungs, that no
  variant overrides `prompt` or `judge`, and 3–5 scenarios per entry. Wire into
  `validate-entry` so a malformed scenario fails the same way a malformed corpus
  does.
- Extend `map-a-site` (P2.6) with a scoring step, or add a sibling `score-a-map`
  skill: read the corpus, propose one scenario per archetype grounded in real view
  and component names, and refuse to write a rubric check that mentions sightmap.
- Hand-author scenarios for 3 seed entries as the reference set, styled on
  `netlify/context-and-tools` — specific checks, negative checks, vacuous-pass
  allowances where a sandbox limit is a legitimate outcome.
- Acceptance: an agent given only the skill and an entry slug produces scenarios
  that pass validation and a human review against SPEC §3.2 with no edits.

## P6.3 Result record + index — atlas

- `scripts/axis-record`: reads an AXIS `report.json`, computes per-rung medians,
  spreads, both lifts, and `confidence` per SPEC §6, and writes
  `entries/<slug>/axis/result.json` + `history/<date>.json`. All seven
  publishability gates enforced here, in code, not in review.
- `gen-index` folds the §4.2 projection into each entry, computing `current` from
  the entry commit. Entries without `axis/` are untouched.
- `schema/entry.schema.json` and atlas `docs/SPEC.md` updated for the `axis` block
  in the same PR.
- Acceptance: fixture `report.json` in → exact expected `result.json` and index
  block out; fixtures with 2 repetitions, a self-judged run, a
  goal-achievement-flat lift, and an overlapping spread each produce the correct
  refusal.

## P6.4 Scoring workflow — atlas

- `.github/workflows/axis.yml`: monthly schedule plus `workflow_dispatch` plus an
  `axis-score` label on an entry PR. Never on every merge — SPEC's cost profile is
  tens of agent runs per entry, and P6.0 will have made that concrete.
- Per-run budget cap and a hard scenario token limit; the workflow fails loudly
  rather than silently truncating the matrix.
- Publishes the HTML report to a durable URL, commits `result.json` and the
  history record, and lets `publish.yml` carry the index block onward.
- Respects tiers (SPEC §7): `live` entries are scored serially from one region,
  with a scenario's rungs kept contiguous (SPEC §2.1).
- Provisioning needed from a maintainer, same list as P5: a scoring API key as a
  repo secret billed to a service account (not an individual subscription), a
  monthly budget ceiling, and sign-off on the canonical agent and model.
- Acceptance: dispatch against one seed entry produces a committed record, a live
  report URL, and an index block, inside the stated budget.

## P6.5 Gallery surface — sightmap/sightmap `web/`

- Card badge, detail scorecard (the three-rung ladder, per-dimension deltas,
  `lift_total` labelled as the whole-stack comparison), stale handling, sort
  behaviour, machine twins, and the `llms.txt` clause, all per SPEC §5.
- Build against a fixture entry carrying an `axis` block before real scores exist,
  and against one that carries none — the unscored path is the common case and
  gets the same test coverage as the scored one.
- Same constraints the gallery already lives under: no network at build or run
  time, no third-party requests, prerendered per route.
- Acceptance: prerendered pages for scored, unscored, and stale entries; a lift
  sort that places unscored last; `pnpm build` and `pnpm test` green; browser check
  at 1280×900 and 390×844.

## P6.6 Regression detection + freshness — atlas

- `axis baseline set` per entry after its first `clear` measurement; baselines
  committed. Scheduled runs use `--compare-baseline`.
- A lift regression beyond noise adds the entry to the P5.1 rolling staleness
  issue, since the usual cause is selectors that no longer match. This is the
  payoff of SPEC §6's "publish negative lift" rule.
- A harness version bump invalidates every baseline: the PR that bumps
  `@netlify/axis` re-baselines in the same change and says so.
- Acceptance: a deliberately broken selector in a copy of a seed entry produces a
  detected regression and an issue update.

## P6.7 Launch content — both

- A methodology page on sightmap.org that is a fair reading of SPEC §2, §6, and §7
  — including what we expect *not* to move, why the middle rung exists, and why a
  negative lift gets published.
- A blog post walking one entry end to end with its real report linked. Lead with
  the measurement, not the marketing: the credibility of every number on the site
  rests on the first one being reported honestly, negatives included.
- Worth raising with Netlify before publishing: no one has benchmarked agent
  experience for general browser use, the §2.4 finding is useful to them, and a
  joint framing is better than a vendor claim about its own product.
- README badge once AXIS ships its embeddable badge (on their roadmap, not ours).

---

## Risks

**Cost.** 4 scenarios × 3 rungs × 5 repetitions is 60 agent runs per entry, each
driving a browser — the third rung raised this by half over a two-variant design,
and it is worth it. This is the dominant constraint and the reason for a schedule
rather than per-merge scoring, hard limits, and a budget ceiling in P6.4. P6.0
replaces this estimate with a measurement.

**Variance.** Agents are stochastic and live sites are worse. Mitigated by
medians, published spreads, and SPEC §6's refusal to headline a lift inside the
noise. If P6.0 shows spread swamping lift at n=3, either n rises or the phase
stops.

**AXIS is being used past its design centre.** Per Sean, it is built for auditing
whether a service is set up so an agent *can* do well, not for scoring browser
operation. The concrete consequence — the site under test barely reaching the
Service dimension — is SPEC §2.4 and is a named P6.0 finding. If it turns out
worse than described, the honest outcomes are to report per-dimension rather than
composite, or to take it upstream, not to reweight around it.

**Measuring our own product.** Third-party judge, default weights, committed
scenarios and reports, published negatives, the `with-skills` middle rung, and the
§6.6 goal-achievement gate are all load-bearing. Dropping any one of them turns
this from evidence into marketing.

**Live-site exposure.** Read-only tasks, serial execution, one region, POLICY.md
in force. A site that signals it does not want automated traffic gets no `live`
score and no workaround.

**Harness drift.** AXIS is young and moving — a judge-model change or a scoring
tweak can move every number with nothing about the maps changing. The version is
pinned, recorded in every result, and a bump re-baselines.

**Scope creep into the spec.** This phase adds nothing to `spec/v1/` and no CLI
verb. If an implementation task starts wanting one, that is a signal the design
drifted, not a small extra PR.

## Open questions for maintainers

1. **Canonical agent and model.** One pair defines published scores. Claude Code
   is the obvious default; Netlify's own config tests three agents and judges with
   `codex`. Naming a second agent multiplies the cost model.
2. **Who pays, and what is the ceiling?** P6.4 cannot be built without a number,
   and P6.0 needs a small budget of its own.
3. **Do `live` scores launch at all,** or does the first release publish only
   `self-hosted` and our own properties until variance is understood?
4. **Per-scenario detail public?** `result.json` carries it; the gallery currently
   shows only the aggregate. Publishing per-scenario numbers is more honest and
   more gameable.
5. **Does a scored entry ever become a quality bar?** SPEC §8 says no at launch.
   Worth an explicit decision rather than drift.
6. **Track A on its own merits?** Scoring the CLI and skills needs no browser and
   no live sites, and would be useful whether or not Track B proceeds.
