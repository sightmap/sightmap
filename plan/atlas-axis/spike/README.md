# P6.0 spike kit

Runnable harness for the feasibility spike in [`../PLAN.md`](../PLAN.md),
implementing the contracts in [`../SPEC.md`](../SPEC.md). **Throwaway:** nothing
here merges into the atlas repo as-is; what survives is the findings report.

32 jobs: 2 sites (`sightmap-org`, `airbnb`) × 2 scenarios × the three-rung
ladder (`no-context` ×2, `with-skills` ×3, `with-sightmap` ×3), single
`axis run`, one report.

## Prerequisites

- Node 22+.
- `npm install -g @sightmap/sightmap` — jobs invoke the CLI from PATH; the
  managed Chrome installs once via `beforeAll` (`bin/install-chrome.sh`).
- The canonical agent: Claude Code CLI with `ANTHROPIC_API_KEY` set (a service
  key, not a personal login — SPEC §3.5).
- A judge that is not the agent under test: `codex` (or `gemini`) installed with
  its key. AXIS validates this at pre-flight and the run will not start without
  it. Do not weaken `judging` to self-judge except for a mechanical smoke run,
  whose report is not a result.
- Open egress. A proxied/allowlisted environment (e.g. a CCR container) can
  build and validate this kit but cannot run the live-site jobs — see
  `../spike-findings.md` for what was validated where.

## Run order

```bash
cd plan/atlas-axis/spike
npm ci

# 1. Bot-defense pre-flight FIRST (PLAN P6.0): is each candidate site even
#    scenario-able from this egress? Records served/suspect/walled per site.
sightmap browser install            # once; preflight defaults to this chrome
./preflight/preflight.sh

# 2. The matrix. ~3–6h wall-clock at concurrency 2 if limits are hit.
npx axis run

# 3. Aggregate: per-rung medians + lifts next to raw ops metrics.
node analyze.mjs

# 4. The full visual report (per-check judge evaluations, transcripts):
npx axis reports latest --html
```

To iterate on one scenario first: `npx axis run -s "sightmap-org/02-extract@*"`.

## What to record (the P6.0 questions, PLAN)

1. **Does the ladder separate?** `with-sightmap` − `with-skills` vs. spread, per
   site; wider on airbnb than sightmap-org?
2. **Does the middle rung behave?** `with-skills` should land between the other
   two. Level with `with-sightmap` ⇒ the tooling is doing the work, not the map.
3. **Service dimension** — confirm browser work classifies as Environment
   (SPEC §2.4) and record what an empty Service dimension scores (SPEC §2.6).
4. **Judge honesty** — human-grade all 32 jobs against the rubrics; report
   judge–human agreement per rung (SPEC §2.6 legibility bias).
5. **Mechanics** — Chrome symlink across isolated HOMEs; does AXIS's `skills`
   field reach the agent (fallback: `sightmap skills install --target
   "$HOME/.claude/skills"` as a setup step); does the agent drive
   `sightmap browser` unprompted in the `with-sightmap` rung; do the 8/12-minute
   limits fit.
6. **Cost** — tokens + wall-clock per job from `analyze.mjs`, extrapolated to a
   48-run entry and a full-atlas sweep.

Findings go to `../spike-findings.md`, then to the Netlify sync (PLAN).

## Layout

```
axis.config.ts        harness config per SPEC §3.5 (pinned, judged, capped)
helpers/variants.ts   the three-rung ladder, reps expanded as variants
scenarios/<site>/     2 scenarios per site, rubrics per SPEC §3.2
fixtures/<slug>/      vendored corpora (from the PR #172 tree; production
                      ladder uses `sightmap atlas add` instead — SPEC §3.4)
bin/                  beforeAll chrome install + per-job HOME symlink
preflight/            bot-defense check, run before anything else
analyze.mjs           report aggregation: AXIS numbers next to raw ops metrics
```
