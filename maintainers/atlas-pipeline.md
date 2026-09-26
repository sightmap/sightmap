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
   `sightkick`, `intent`, `nominate`, `rescan`, plus the optional `claim` (see
   `web/src/lib/submit-types.ts`). The email is hashed with `ATLAS_SUBMIT_SALT`
   and never leaves the function as plaintext.
2. **Function.** `web/netlify/functions/atlas-submit.mts` preflights the URL
   (`web/scripts/lib/preflight.ts`), refuses a quarantined host, checks the
   claim if one was sent, rate-limits, and hands the submission to whichever
   runner `ATLAS_RUNNER` names — `netlify`, `github`, or `queue`.
3. **Runner.** A coding agent (or, on the GitHub path, the workflow) installs
   the sightmap CLI and a Chrome build, runs `pnpm atlas:intake`, and gets a
   listing YAML, a dated scan JSON, and a summary. It then runs
   `pnpm atlas:card` to add what the scan saw to the submitter's card, if the
   host has one.
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
8. 🖐 **Email the owner.** By hand, using the template below.

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

## Runner

Netlify Agent Runners are the runner in use. A run executes a coding agent
against a checkout of this repository on Netlify's infrastructure, on an
automated branch off the production branch. `atlas-submit.mts` starts one by
calling the same REST endpoint the Netlify CLI calls.

Setup:

1. Create a Netlify personal access token (User settings → Applications).
2. Set the environment variables below on the site (Site configuration →
   Environment variables).
3. On each submission the function POSTs
   `https://api.netlify.com/api/v1/agent_runners?site_id=<site_id>` with
   `Authorization: Bearer <token>` and JSON `{prompt, agent, model?, branch?}`,
   and gets back `{id, state: 'new', …}`. The prompt points the agent at
   `web/ATLAS_PIPELINE.md` and carries the submission fields.

| Variable | Value |
|---|---|
| `ATLAS_RUNNER` | `netlify` |
| `NETLIFY_AGENT_TOKEN` | The personal access token |
| `NETLIFY_SITE_ID` | The site id. Falls back to `SITE_ID`, which Netlify injects at runtime |
| `ATLAS_RUNNER_BRANCH_BASE` | Optional. The branch a run starts from; omitted from the request when unset |
| `ATLAS_RUNNER_MODEL` | Optional. Omitted from the request when unset |
| `ATLAS_DAILY_RUNS` | Optional, default 20. Ceiling on runner triggers per UTC day across all submitters. Agent Runner concurrency is capped per plan (Free 1, Personal 3, Pro 10, Enterprise 50); over the ceiling a submission is stored `queued`, with no run |
| `ATLAS_SUBMIT_SALT` | Per-deploy salt for the email hash |
| `NETLIFY_AUTH_TOKEN` | Personal access token the runner uses to write the `atlas-try` store from `pnpm atlas:card`. Without it (or without `NETLIFY_SITE_ID`) the card step prints one notice and exits 0 |

`NETLIFY_SITE_ID` and `NETLIFY_AUTH_TOKEN` are also the two repository secrets
the GitHub fallback workflow needs, for the same step.

A run appears in the Netlify dashboard with its state, a log, and a Deploy
Preview at `https://agent-<run-id>--<site>.netlify.app`. When the agent has
committed the listing, open the pull request from the run; that PR and its
Deploy Preview are what a maintainer reviews in step 5.

The same operations from the CLI:

```sh
netlify agents:create "<prompt>" --agent claude [--branch <base>] [--model <model>] [--json]
netlify agents:list
netlify agents:show <run-id>
netlify agents:stop <run-id>
```

### Runner requirements

A runner environment must provide:

- **A Chrome build.** Either `sightmap browser install` can download a Chrome
  for Testing build, or `ATLAS_CHROME_PATH` points at an existing one. With
  neither, the agent stops and reports.
- **Outbound HTTPS to arbitrary hosts.** A scan loads a third-party site, not a
  Netlify one. Where a proxy is required, `HTTPS_PROXY` must be set; the scanner
  forwards it into Chrome.
- **Time for the scan.** Roughly two minutes per scan, plus the CLI install and
  the build, within the runner's own time ceiling.

If the Chrome install or the outbound egress fails, set `ATLAS_RUNNER=github`.

### GitHub Actions fallback

With `ATLAS_RUNNER=github` the function fires a `repository_dispatch`
(`event_type: atlas-submission`, `client_payload` `{submission_id, url, intent,
owner, sightkick, nominate, rescan, email_hash}`) that
`.github/workflows/atlas-review.yml` picks up; that workflow also has a
`workflow_dispatch` trigger, so a maintainer can run a submission by hand from
the Actions tab. `ATLAS_GITHUB_TOKEN` is a classic PAT with `repo` scope, or a
fine-grained token with `contents: write` and `actions: write` on
`ATLAS_GITHUB_REPO` (`owner/repo`, default `sightmap/sightmap`), and
`ANTHROPIC_API_KEY` is a repository secret — without it the review step falls
back to heuristics, so the drafted description and category need closer
attention. `NETLIFY_SITE_ID` and `NETLIFY_AUTH_TOKEN` are two more repository
secrets, read only by the launch-card step; without them that step prints a
notice and the run carries on. The repository variable `ATLAS_RUNNER_LABEL` selects the machine:
leave it unset for
`ubuntu-latest`, or set it to a Namespace label such as
`nscloud-ubuntu-24.04-amd64-4x8-with-cache` to run on a Namespace runner.

## Blob stores

Four, all on the site's own Netlify Blobs. Two are the submission pipeline's
own bookkeeping; two back the unlisted cards.

| Store | Key | Value |
|---|---|---|
| `atlas-submissions` | `<yyyy-mm-dd>/<id>`, plus `index/<id>` pointers | One submission record. The only copy of a submitter's email address anywhere |
| `atlas-rate` | hashed client IP, plus `runs/<yyyy-mm-dd>` | The per-IP window and the global daily runner ceiling |
| `atlas-try` | `<host>` | The record behind `/try/<host>`: the claim date, the submitted URL, and the scan once the runner adds one. Written by the submit function on a verified claim, updated by `pnpm atlas:card`, and expired 30 days after the last scan |
| `atlas-quarantine` | `<host>` | `{ at, reason? }`. Presence is the whole signal |

The host key is the canonical host everywhere — lowercase, no port, no leading
`www.` — the same string `canonicalHost()` in `web/scripts/lib/directory.ts`
produces and that `/atlas/hosts/<host>.json` is keyed by.

### Quarantine

Refuses a host without deleting anything. `/try/<host>` answers 410 and the
submit endpoint answers 403 `quarantined`, with or without a claim.

```sh
netlify blobs:set atlas-quarantine example.com '{"at":"2026-09-10T00:00:00Z","reason":"owner request"}'
netlify blobs:get atlas-quarantine example.com
netlify blobs:delete atlas-quarantine example.com   # lifts it
```

Use it for a host that must stop being served now — an owner's removal request
that arrives before a card expires, a site that turned into something else
after its scan. It does not touch a merged Atlas listing: that is a takedown,
below.

### Domain claims

A submission may carry a `claim`: the 32 hex characters the owner published as
a comment line `# sightmap-claim: <token>` in `https://<host>/webmcp.txt`. The
function fetches that file (at most three same-site redirects, 5 s, 64 KiB) and
compares. A match stores `claim: { verifiedAt }` on the submission, writes the
`atlas-try` record, and returns the card URL. A failure is 422
(`claim-unreachable` or `claim-mismatch`) and stores nothing at all, so the
owner can fix the file and retry without spending one of their five daily
submissions.

The token is generated by the owner and only ever compared. Sightmap does not
issue it, store it, or log it, so there is nothing to rotate and nothing to
leak — and a claim is domain control, not a review. Nothing about a card says
a site is safe, endorsed, or listed.

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

Any visitor-facing count, such as "Recommended by N visitors", is maintained by
hand in the listing YAML, so the diff is the audit trail; there is no vote
endpoint.

## Takedown

Delete the listing and its scans, then rebuild:

```sh
git rm web/src/data/directory/<slug>.yaml
git rm -r web/src/data/directory/scans/<slug>
```

If the site also has an unlisted card, delete its record, or quarantine the
host when you want the URL to keep answering with a removal notice:

```sh
netlify blobs:delete atlas-try <host>
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
by hand with the same `url`. Reasonable cadence: on owner request, before
featuring a listing more than a couple of months old, and whenever a `sensitive`
tool appears in a drift block — that last one is worth a fresh human read of
every description, not just the new ones.

Do not re-litigate a description a maintainer already approved unless the site
actually changed.

## Metrics worth watching

Limited to numbers a maintainer would act on:

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

None of these is a target. They indicate whether the pipeline is still doing
what it was built for.
