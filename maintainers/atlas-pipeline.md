# The Atlas listing pipeline

How a submitted URL becomes a listing on sightmap.org/atlas, who does what, and
what is deliberately still a human step.

This is the operator doc. The instructions the *review agent* follows live in
[`../web/ATLAS_PIPELINE.md`](../web/ATLAS_PIPELINE.md); the data contract lives
in [`../web/src/data/directory/README.md`](../web/src/data/directory/README.md).
When those two and this one disagree about a command or a field name, they win
— they sit next to the code.

## What the Atlas is now

Three kinds of entry, on one page, with a type filter:

| Kind | What it is | Where it comes from |
|---|---|---|
| **live** | A real product surface that exposes WebMCP tools | Submission → scan → listing YAML |
| **demo** | A hackathon entry, competition submission, example app, or experiment | Same pipeline, `type: demo` |
| **community map** | A vendored sightmap corpus for a site — useful automation data, not a claim the owner ships WebMCP | `web/src/data/atlas/`, arrives by PR |

The Atlas publishes **facts, not grades**. A listing says how many tools a site
registered, whether each has a description and an input schema, whether the
tool set changes with the page, and what the scanner classified each tool as.
There is no score, no star, no "certified", and the scan statuses are
observable outcomes only (`tools-found`, `api-empty`, `api-absent`, `blocked`,
`load-error`, `needs-review`).

Tool kinds — `read` / `action` / `sensitive` — are **classified by Atlas** and
labelled that way everywhere they are shown. The heuristic in
`web/scripts/lib/tool-risk.ts` errs toward the riskier bucket: anything it
cannot place is `action`, so a maintainer's correction only ever relaxes a
label. That is the whole point of the human step.

## End to end

Manual steps are marked 🖐.

1. **Submission.** The form at `/atlas/submit` POSTs to
   `/.netlify/functions/atlas-submit`: `url`, `email`, and the flags `owner`,
   `sightkick`, `intent`, `nominate`, `rescan` (see
   `web/src/lib/submit-types.ts`). The email is hashed with `ATLAS_SUBMIT_SALT`
   and never leaves the function as plaintext.
2. **Function.** `web/netlify/functions/atlas-submit.mts` preflights the URL
   (`web/scripts/lib/preflight.ts`), rate-limits, and hands the submission to
   whichever runner `ATLAS_RUNNER` names — `netlify`, `github`, or `queue`.
3. **Runner.** A coding agent (or, on the GitHub path, the workflow) installs
   the sightmap CLI and a Chrome build, runs `pnpm atlas:intake`, and gets a
   listing YAML, a dated scan JSON, and a summary.
4. **PR.** The runner opens `atlas: list <host>` (or `atlas: rescan <host>`, or
   `atlas: needs review — <host>`) with the summary as the body and a maintainer
   checklist. The agent never merges.
5. 🖐 **Review.** A maintainer works the checklist: read every tool name and
   description, open the site, confirm live vs demo, confirm every tool kind,
   approve the description and category. Optionally run one supervised
   read-only journey and record it. The Deploy Preview on the PR *is* the
   review artifact — you look at the listing page as it will ship.
6. 🖐 **Merge.** Merging is the listing decision. Nothing else is.
7. **Deploy.** Netlify builds `web/` on merge; `scripts/build-atlas.ts` reads
   `src/data/directory/` and regenerates the gallery, the JSON API under
   `/atlas/`, the markdown twins, and the badges. No network call at build time.
8. 🖐 **Email the owner.** By hand, for now — we do not have an automated
   sender and we are in no hurry to build one.

### Owner email template

Send from a maintainer address. Fill every placeholder; delete the Sightkick
paragraph when it does not apply.

```
Subject: {SITE_NAME} is listed on the Sightmap Atlas

Hi {NAME},

{SITE_NAME} is now listed on the Sightmap Atlas:
https://sightmap.org/atlas/{SLUG}

We scanned {URL} on {SCAN_DATE} and found {TOOL_COUNT} WebMCP tool(s) across
{PAGE_COUNT} page(s). The listing shows what we found and nothing else — no
score, no ranking. The scan enumerated your tools; it never called one.

The full scan report is on the page, and the listing itself is a single YAML
file in a public repo, so you can see exactly what we published and open a PR
against it:
https://github.com/sightmap/sightmap/blob/main/web/src/data/directory/{SLUG}.yaml

{SIGHTKICK_PARAGRAPH: If you'd like to add or expand a tool layer, sightkick
generates one from your existing UI — https://sightmap.org/sightkick}

If anything is wrong, or you'd rather not be listed, reply to this email and
we'll correct or remove it. Removal is a deletion, not a flag.

— {MAINTAINER}, Sightmap
```

## Runner options

### Netlify Agent Runners — primary

Netlify Agent Runners run an AI agent (Claude Code, Codex, Gemini, or OpenCode)
against the site's repository on Netlify's infrastructure. Verified behaviour:

- The run works on an **automated branch off the production branch** and
  produces a **Deploy Preview** at `https://agent-<run-id>--<site>.netlify.app`.
- From the run you can **optionally open a pull request**; merging it publishes.
- Runs start from the Netlify dashboard, from a deploy's page, from Linear,
  from the CLI, or from the REST endpoint the CLI uses.
- **Concurrency is per plan:** Free 1, Personal 3, Pro 10, Enterprise 50.
- Webhook triggers are **"coming soon"** — which is exactly why
  `atlas-submit.mts` calls the REST API itself rather than waiting for one.

Configure:

1. Create a Netlify **personal access token** (User settings → Applications).
2. Set on the site (Site configuration → Environment variables):
   `ATLAS_RUNNER=netlify`, `NETLIFY_AGENT_TOKEN=<the token>`,
   `NETLIFY_SITE_ID=<site id>`, `ATLAS_RUNNER_BRANCH_BASE` (the production
   branch the run branches from), `ATLAS_RUNNER_MODEL` (optional), and
   `ATLAS_SUBMIT_SALT` for the email hash.
3. The function POSTs
   `https://api.netlify.com/api/v1/agent_runners?site_id=<site_id>` with
   `Authorization: Bearer <token>` and JSON `{prompt, agent, model?, branch?}`,
   and gets back `{id, state: 'new', …}`. The prompt points the agent at
   `web/ATLAS_PIPELINE.md` and carries the submission fields.

By hand, the same thing from the CLI:

```sh
netlify agents:create "<prompt>" --agent claude [--branch <base>] [--model <model>] [--json]
netlify agents:list
netlify agents:show <run-id>
netlify agents:stop <run-id>
```

A run appears in the dashboard with its state, a log, and its Deploy Preview.
When the agent has committed the listing, open the PR from the run — that is
the artifact a maintainer reviews.

**Known unknowns, to confirm on the first real run.** Note the answers here
when you have them:

- **Can the runner install Chrome?** `sightmap browser install` downloads a
  Chrome for Testing build. Whether the agent runner environment permits that
  download is not documented. Treat it as unknown until a run proves it. If it
  cannot, the playbook has the agent fall back to `ATLAS_CHROME_PATH` and, when
  there is no browser at all, stop and report — which is the signal to flip
  `ATLAS_RUNNER` to `github`.
- **Network egress.** The scan needs to reach an arbitrary third-party site.
  Confirm that outbound HTTPS to a non-Netlify host works and whether it goes
  through a proxy (the scanner forwards `HTTPS_PROXY` into Chrome).
- **Run duration.** A scan is budgeted at roughly two minutes plus install and
  build. Confirm the runner's own ceiling before assuming a rescan sweep fits.

### GitHub Actions on a Namespace runner — fallback

Set `ATLAS_RUNNER=github`. The function fires a `repository_dispatch` with
`event_type: atlas-submission` and `client_payload`
`{submission_id, url, intent, owner, sightkick, nominate, rescan, email_hash}`,
which `.github/workflows/atlas-review.yml` picks up. The same workflow has a
`workflow_dispatch` trigger, so a maintainer can run a submission by hand from
the Actions tab.

- `ATLAS_GITHUB_TOKEN`: a classic PAT with `repo` scope, or a fine-grained
  token with **contents: write** and **actions: write** on
  `ATLAS_GITHUB_REPO`.
- `ATLAS_GITHUB_REPO`: `owner/repo`.
- `ANTHROPIC_API_KEY` as a repository secret. Without it the review step falls
  back to heuristics — the listing is still written, but the drafted
  description and category need more of your attention.
- Repository variable `ATLAS_RUNNER_LABEL`: leave unset for `ubuntu-latest`;
  set it to `nscloud-ubuntu-24.04-amd64-4x8-with-cache` to run on a Namespace
  runner. Namespace labels are `nscloud-<os>-<arch>-<shape>[-with-cache]`, or
  `namespace-profile-<name>` for a named profile; add
  `namespacelabs/nscloud-cache-action@v1` for pnpm store caching there.

The workflow always uploads the summary and the scan JSON as an artifact, even
when the scan fails — a `blocked` or `load-error` result is the case where
those files matter most.

### LangSmith Managed Deep Agents — an option we have not built

LangSmith's Managed Deep Agents is a hosted, API-first agent runtime: `mda init`
/ `dev` / `deploy`, then REST calls to create a thread and run it. It is in
public beta, US-only, and offers a sandbox. It is listed here as the third
option and nothing more — **no code in this repo targets it.**

What adopting it would take: deploy the agent (`mda deploy`) with the playbook
as its instructions; add an `ATLAS_RUNNER=mda` branch to `atlas-submit.mts` that
creates a thread and starts a run; give the sandbox a checkout of this repo plus
a Chrome build; and replace the "agent opens the PR" step, since there is no
Netlify branch or Deploy Preview — the run would have to push a branch and open
the PR through the GitHub API itself, and we would lose the preview that is
currently the review artifact. Revisit only if the Netlify path runs out of
concurrency and the Actions path proves too slow.

## Safety controls

Before a scan:

- 🖐 **Manual preflight** by the reviewing agent or maintainer: HTTPS, public
  hostname, no credentials in the URL, redirect stays on-origin, production vs
  demo, ownership claimed (a claim, not a verified fact), and the stated intent
  is not dangerous.
- **HTTPS only.** `preflightUrl()` rejects everything else. `allowLocal` exists
  for loopback fixtures in tests and is never set from a submission.
- **DNS-private rejection.** `resolvePublic()` resolves the host and rejects any
  A/AAAA record in a private or reserved range — the literal check cannot catch
  `internal.example.com → 10.0.0.5`, this can. A resolver failure is also a
  rejection.

During a scan:

- **Fresh profile.** Each scan gets a throwaway directory, its own
  `--sightmap-dir`, and a profile that is deleted with it. There is no account
  in it and there never will be.
- **No tool execution.** The scanner enumerates registrations and stops. No
  `mcp call`, no form submit, no click-through.
- **Timeouts.** Per-page navigation and a whole-scan budget; the scan records
  "budget exhausted" rather than running long.
- **Same-origin only.** At most three pages, all on the submitted origin. An
  off-origin redirect on the first load ends the scan as `blocked`. Links that
  look like logout, delete, cancel, checkout, purchase, or a file download are
  never followed.
- **Denied permissions.** Chrome runs with permission prompts denied, so a page
  asking for camera, location, or notifications gets a refusal, not a hang.
- **Size caps.** 200 tools, 20 KB per input schema (larger ones are recorded as
  truncated), 1500 characters of description before it is flagged.

After a scan:

- 🖐 **A human reads every tool name and description.** Not a sample. Every one.
  This is the step the pipeline exists to make cheap.
- **Tool output is never rendered as HTML.** Descriptions and names are
  untrusted text from the scanned site; suspicious wording flips the scan to
  `needs-review` and is quoted inside code fences, never obeyed and never
  passed through a raw-HTML renderer. (Same discipline as the community atlas
  READMEs, which go through the hardened `marked` instance in
  `web/scripts/lib/atlas.ts`.)
- **Journeys are read-only and supervised.** No authentication, no forms, no
  checkout, no writes, and a maintainer watching.

## Recording a supervised journey

Optional, and always by hand. A journey is one maintainer, one intent, one
read-only run — never automated and never part of intake.

```sh
sightmap browser start --url https://example.com/
sightmap browser mcp list                       # what is registered here
sightmap browser mcp call search --param q=pricing
sightmap browser stop
```

Only call tools you have classified `read`. If a call would write, buy, send,
post, or log in, you do not make it — record `outcome: failed` with a note
saying the intent was not reachable read-only. Then add to the listing YAML:

```yaml
journey:
  intent: Find the pricing page
  outcome: passed          # passed | failed
  ran_at: 2026-09-09
  tools_called: [search, get_page]
  duration_ms: 4200
  notes: Ran headless with no account; two tool calls.
```

`notes` is where you say what a reader could not infer: whether you needed an
account (you should not have), what the tool returned, what surprised you.

## Editorial labels and collections

Both are maintainer-only fields on the listing, both default to empty, and
neither is ever set by the review agent.

| Label | Means |
|---|---|
| `promising` | Worth watching; a maintainer liked what the tools do |
| `verified` | A maintainer ran a supervised journey and it did what the listing says |
| `featured` | Chosen for the front of the gallery |

`collections` are just named groupings — `competition-demos`,
`built-with-sightkick`, `new-this-week`. Keep the vocabulary small; a
collection nobody browses is a maintenance cost.

If we ever show **"Recommended by N visitors"**, it is maintained by hand: the
count lives in the listing YAML, a maintainer edits it in a PR, and the diff is
the audit trail. We do not ship a vote endpoint to find out that vote endpoints
get gamed. If the number ever appears without a git history behind it, that is
a bug.

## Takedown

Delete the listing and its scans, then rebuild:

```sh
git rm web/src/data/directory/<slug>.yaml
git rm -r web/src/data/directory/scans/<slug>
```

Merge. `scripts/build-atlas.ts` regenerates the gallery, the JSON API, the
markdown twin, and the badge from what is left; nothing is fetched at build or
run time, so the removed entry is gone from the next deploy. Note that
`/atlas/` output is deliberately *not* marked immutable in `web/netlify.toml`
precisely so a takedown is not pinned in caches for a year.

An owner asking for removal gets it, without an argument and without a
justification requirement. Reply, delete, confirm.

## Rescans

A rescan reuses the slug and adds a new dated report under `scans/<slug>/`;
old reports are kept forever. The listing's `drift` block shows which tools
were added and removed since the previous scan, and the PR title is
`atlas: rescan <host>`.

Trigger one by resubmitting the URL with `rescan`, or by running the workflow
by hand with the same `url`. Reasonable cadence: on owner request, when a
listing is more than a couple of months old and we are about to feature it, and
whenever a `sensitive` tool appears in a drift block — that last one is worth a
fresh human read of every description, not just the new ones.

Do not re-litigate a description a maintainer already approved unless the site
actually changed.

## Metrics worth watching

Trimmed to what we would actually act on:

- **Supply** — listings merged per week; share arriving as `owner` vs
  `nominator`; how many submissions never become a listing, and why.
- **Product** — scan success rate (`tools-found` as a share of all scans);
  median time from submission to merged PR; how often a maintainer had to
  correct a tool kind (that number going up means `tool-risk.ts` needs work).
- **Quality** — listings with a recorded supervised journey; `needs-review`
  rate; takedown requests.
- **Viral loop** — sites that arrive with no tools and later resubmit with a
  Sightkick-built layer; badge fetches from `/atlas/<slug>/badge.svg`; inbound
  links from listed sites.

None of these is a target. They are the numbers that tell us the pipeline is
still doing what we built it for.

## How this differs from webmcp.com

Both index sites that expose WebMCP tools. The designs differ; this is what
ours does, stated plainly.

| Dimension | webmcp.com | Sightmap Atlas |
|---|---|---|
| Verdict | Assigns a grade | Publishes factual checks — counts, yes/no indicators, no score |
| Verification | One-shot agent test | Replayable Sightkick verification transcript, plus a stored supervised journey when a maintainer ran one |
| Tool listing | One flat list per site | Tools grouped by the page or view they register on |
| Improving a site | A suggestion snippet | A drafted `.sightkick` tool layer plus a full agent prompt |
| Intake | Live scanner form | Maintainer-approved PR pipeline; the Deploy Preview is the review artifact |
| Coverage | Platform-derived inventory, broad | Small, legible, inspected directory |
| Moderation | Happens out of view | Every listing is a git diff; takedown is a deletion in public history |

The trade is deliberate and it cuts both ways: we will always have fewer
entries, and every entry will have a name attached to the decision to publish
it.
