# P6.0 spike — findings log

Running record for the feasibility spike ([`PLAN.md`](PLAN.md) P6.0). Appended
per session; newest section first. The go/no-go writeup will be the final
section.

## 2026-08-07 — kit built, mechanics validated, no scored jobs yet

Environment: a sandboxed CI-like container (proxied egress, no agent API keys).
Everything that does not require open egress or an agent key was exercised for
real; the scored 32-job matrix needs a machine with both.

### What ran, and passed

- **Harness pins.** `@netlify/axis@1.17.2` (current) installed and pinned;
  `@sightmap/sightmap@0.18.0` installed globally; managed Chrome for Testing
  151.0.7922.77 downloaded and installed by `sightmap browser install`.
- **Config chain.** `axis.config.ts` loads, `beforeAll` fires, and pre-flight
  validation proceeds in order. The run correctly **refused to start** because
  the judge (`codex`) had no key — which is the SPEC §3.5 third-party-judge
  requirement being *enforced by the harness*, not just by our convention.
- **The matrix.** All four scenario modules load through jiti (the same loader
  AXIS uses — `check-matrix.mjs`): 32 jobs, rung distribution
  `no-context ×2 / with-skills ×3 / with-sightmap ×3` per scenario, judge
  weights summing to 1, and no variant overriding `prompt` or `judge`
  (SPEC §3.2 rules 1–2 hold by construction).

### Mechanical findings (the kind the spike exists to catch)

1. **AXIS scrubs `HOME` from the `beforeAll` environment.** The documented
   passthrough list (PATH/USER/SHELL/LANG/TERM/TMPDIR) is real and `HOME` is not
   on it, so `sightmap browser install` — which writes to `~/.sightmap/` — died
   inside `beforeAll`. Fixed in `spike/bin/install-chrome.sh` by deriving the
   real home from the passwd entry. Worth mentioning to Sean: any lifecycle
   script that shells a tool expecting `$HOME` will hit this.
2. **A pre-flight script can lie in a sandbox, convincingly.** The first
   bot-defense pre-flight reported all seven candidate sites as SERVED — with
   suspiciously identical ~187.8 KB payloads. Inspection showed Chrome's own
   `ERR_CONNECTION_RESET` error page for every site: the container routes
   traffic through an HTTP proxy Chrome knows nothing about, so Chrome has **no
   egress at all** here. `preflight.sh` now fingerprints Chrome's neterror
   payload and reports `NO-EGRESS` instead of a false SERVED, and warns that
   identical byte counts across unrelated sites mean you are measuring your
   egress, not the web.
3. **Chrome-once works.** Install lands in `~/.sightmap/browsers/<version>/…`;
   `preflight.sh` resolves the binary via `sightmap browser install`'s
   idempotent-print behaviour. The per-job symlink (`bin/link-chrome.sh`)
   remains untested until real jobs run.

### Blocked here, runnable on a dev machine

| Needs | Why it is blocked in this container |
|---|---|
| Real bot-defense pre-flight verdicts | no browser egress (`NO-EGRESS` on all seven sites, including sightmap.org) |
| The scored 32-job matrix | no `ANTHROPIC_API_KEY`; no `codex`/`gemini` for the judge |
| Chrome symlink across isolated HOMEs, skills-field reachability, judge honesty, cost numbers | all downstream of the above |

### Next session (a machine with keys + open egress)

```bash
cd plan/atlas-axis/spike
npm ci
npm install -g @sightmap/sightmap && sightmap browser install
./preflight/preflight.sh        # real verdicts; pick airbnb or first fallback
npx axis run                    # the 32 jobs
node analyze.mjs                # rung medians + lifts next to raw ops metrics
npx axis reports latest --html  # for the human-grading pass
```

Then: human-grade all 32 jobs (judge-agreement table, SPEC §2.6), fill the
P6.0 question list (spike `README.md`), and write the go/no-go here.
