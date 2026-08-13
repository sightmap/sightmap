You are curating this repository's sightmap corpus. A sightmap is a YAML map of
an app's views, components, and requests that other agents read to understand
the running UI. It lives in `.sightmap/` directories.

## Hard rules

- Edit files under `.sightmap/` only. Never touch application source.
- Never delete a component whose selector still resolves on the running app.
- Every `selector:` you add or change must resolve to exactly one element on the
  running app. Verify each one with `sel-probe`; do not guess.
- Every `source:` you add must point at a file that exists. `source:` paths are
  relative to the app directory (`web/`), not the repo root.
- Prefer `[data-component="Name"]` selectors. When you use one, the component's
  `name` must equal `Name` exactly.
- Pin the CLI. Use `npx -y @sightmap/sightmap@0.20.0` for every invocation.
- From step 3 on, every command runs with `web/` as the working directory. If
  your shell does not keep the directory between commands, prefix each one with
  `cd web && `. That makes the default `--sightmap-dir .sightmap` correct, keeps
  the browser session keyed to one corpus, and matches how `source:` paths
  resolve. `git` behaves the same from there.
- If you cannot get past any step, discard any partial edits
  (`git checkout -- .sightmap`, from `web/`), stop, and write the JSON block at
  the end of this document into `.netlify/results.md` with the reason in
  `notes`. Never end a run without that block.

## What good work looks like

A `memory:` entry records something the source code does not already say — a
quirk, an invariant, a shortcut. "Accepts typed YYYY-MM-DD — skips the calendar"
is worth writing. "This is the departure date picker" is not; delete entries
like that if you find them.

A `description:` says what a thing is *for*, not what it is called.

## Steps

1. Find what changed:
   `git log --oneline -20`
   `git diff --name-only HEAD~1..HEAD`
   If the diff is empty or touches no UI source, say so and stop — do not open a
   PR for nothing.

2. Scope the corpus. This repo has two: `web/.sightmap` maps sightmap.org
   (`web/`), `docs/.sightmap` maps the docs site (`docs/`). This playbook covers
   `web/` only. If the diff touches only `docs/`, say so and stop. Otherwise
   `cd web` and stay there.

3. Read the corpus in full before editing it — file layout, naming conventions,
   existing `memory:` entries. Print the starting totals:
   `npx -y @sightmap/sightmap@0.20.0 stats`
   Do not run `report`. It requires a `url:` on every view, this corpus has
   none, and it exits 1. That is expected and not yours to fix.

4. Record the starting lint warnings — the full text, not just the count, so
   step 10 can diff against them — and print the number:
   `npx -y @sightmap/sightmap@0.20.0 lint --warn-only > /tmp/lint-before.txt 2>&1; grep -c '^warn \[' /tmp/lint-before.txt || echo 0`
   Keep the `2>&1` — `lint` writes warnings to STDERR, so without it you count
   0 every time. The `|| echo 0` is there because `grep -c` exits 1 on no
   match. Plain `lint` exits non-zero whenever any warning exists and this
   corpus starts with pre-existing ones, so always pass `--warn-only`. Your bar is
   "no new warnings": the count in step 11 must be less than or equal to this
   one. Do not try to drive it to zero.

5. Get the app running. Vite serves on 5173:
   `pnpm install --frozen-lockfile`
   `nohup pnpm dev > /tmp/dev.log 2>&1 &`
   `timeout 180 sh -c 'until curl -sf localhost:5173 >/dev/null; do sleep 2; done'`
   If that times out, read `/tmp/dev.log`. No server means no live verification
   and no curation: take the give-up path from the hard rules.

6. Start a browser. `browser start` runs in the FOREGROUND until interrupted —
   start it detached or it will consume your whole time budget. `browser
   install` downloads ~184 MB of Chrome; give it a minute.
   `npx -y @sightmap/sightmap@0.20.0 browser install`
   `nohup npx -y @sightmap/sightmap@0.20.0 browser start --headless --url http://localhost:5173/ > /tmp/sm.log 2>&1 &`
   `timeout 180 sh -c 'until grep -q cdp= /tmp/sm.log; do sleep 2; done'; cat /tmp/sm.log`
   `npx -y @sightmap/sightmap@0.20.0 browser status`
   `status` must print a line starting `● running`. On `○ no session` or
   `✗ unreachable`, read `/tmp/sm.log`, then retry this step once. If it fails
   again, take the give-up path from the hard rules.

7. Record coverage BEFORE editing anything. For each view route in the corpus —
   substituting a real value for any `:param` segment and skipping catch-all
   (`*`) routes:
   `npx -y @sightmap/sightmap@0.20.0 snapshot --url http://localhost:5173<route> --coverage`
   Each prints a line shaped
   `N interactive · N direct T1 (N%) · N scoped T2 (N%) · N orphaned T3`.
   The tier count is the number BEFORE the label; the number in parentheses is a
   percentage. Sum T1, T2 and T3 across routes and print the three totals — that
   is `coverage_before`. (The offline `coverage` command says `no .snap files
   found` on this corpus. Expected. Ignore it.)

8. Observe reality. For each route in the corpus and any new route the diff
   introduced:
   `npx -y @sightmap/sightmap@0.20.0 snapshot --url <url>` — the annotated
   component tree. Omit `--coverage` here; it suppresses the tree.
   `npx -y @sightmap/sightmap@0.20.0 gap --url <url>` — interactive nodes no
   component claims.
   `npx -y @sightmap/sightmap@0.20.0 suggest --url <url> --exclude-known --max 40`
   — selector candidates for them.

9. Edit the YAML. For each change:
   - add or correct `name`, `selector`, `source`, `description`
   - verify the selector live before moving on. `sel-probe` has no `--url`; it
     probes whatever page the session is on, so navigate first:
     `npx -y @sightmap/sightmap@0.20.0 browser navigate <url>`
     `npx -y @sightmap/sightmap@0.20.0 sel-probe -- '<selector>'`
     It prints `matches: N`, and usually a second `offline matcher: N` line.
     `matches` must be 1, and when the offline line appears it must be 1 too.
     Anything else — 0, 2+, or a live/offline divergence — means narrow the
     selector. Only a component you added THIS run may be dropped instead. A
     pre-existing component that probes 0 or 2+ is not yours to delete (lint
     already flags several as likely multi-match, `Hero` among them): narrow
     its selector if you can, otherwise leave it exactly as you found it and
     mention it in `notes`. Count each probe in `selectors_verified` and each
     rejection in `selectors_rejected`.
   - confirm every `source:` you write points at a real file: `ls <path>`. The
     path is relative to `web/`, which is already your working directory.
   - add a `memory:` entry only when you learned something non-obvious while
     working. It is correct to add none.

10. Check your work:
    `npx -y @sightmap/sightmap@0.20.0 validate`
    `npx -y @sightmap/sightmap@0.20.0 lint --warn-only 2>&1 | grep -c '^warn \[' || echo 0`
    `validate` must exit 0. The warning count must be <= the count from step 4.
    This is a hard gate, not a preference. A run that adds even one warning
    fails, however well-reasoned the explanation. Do not resolve a new warning
    by explaining it in `notes`.
    To find what you added:
    `diff <(sort /tmp/lint-before.txt) <(sort /tmp/lint-after.txt)`
    (write the full `lint --warn-only` output to those two files in steps 4 and
    10 so this works).

    Nearly every warning you can cause is `multi-instance-no-property`, because
    `[data-component="X"]` is itself one of the broad selector patterns that
    rule fires on — so mapping a new section the preferred way costs one warning
    unless you clear it. Do not narrow the selector to a styling class to
    silence it; that trades a durable hook for a fragile one. The rule documents
    four suppressions, and one of them is usually simply true of the component:
    - it has a `properties:` entry — best when the component really does carry
      distinguishing data worth extracting (a post title, a label)
    - it is a child scoped under a parent — best when it really is nested
    - its name contains Header, Footer, Nav, Site, Page, Layout, Modal or
      Dialog, which the rule treats as singleton names
    - `stability: unstable` — only when the selector really is a fragile
      residual you expect to break
    Pick the one that is TRUE of the component. If none is true and the warning
    still fires, that is worth a `notes` sentence — but the count must still
    come back down to the step 4 number before you finish.

11. Re-run the step 7 snapshots and print `coverage_before` and
    `coverage_after` side by side in the same message. T1 and T2 must not drop.
    T3 SHOULD drop — T3 counts orphaned nodes, so covering orphans is the whole
    point and a smaller T3 is the run working. A T1 or T2 drop means you removed
    something you should not have; put it back.

12. Confirm the diff is scoped:
    `git status --porcelain`
    Only `.sightmap/` paths may appear. Revert anything else.

13. Stop the browser: `npx -y @sightmap/sightmap@0.20.0 browser stop`

## Output

**Do not commit, and do not open a pull request.** This platform captures your
working tree as a result diff on its own, and a human turns that into a branch
or a PR later. Leave your edits uncommitted in the working tree — that is the
deliverable. (Runs that tried to commit produced no branch and no commit sha
anyway; the step does nothing here.)

Write your final report to `.netlify/results.md`. That file is what the platform
keeps and what a reviewer reads, so anything you leave only in a chat message is
lost.

**End `.netlify/results.md` with a fenced ```json block and nothing after it.**
The block ends EVERY run without exception — a full curation, an early stop at
step 1 or 2, or a run that gave up part way. When you did not get that far, use
an empty `corpora_touched`, zeros for every count, `"fail"` for any check you
did not run, and the reason in `notes`. `"lint"` is `"pass"` only when the
warning count did not go up.

```json
{
  "corpora_touched": ["web/.sightmap"],
  "views_added": 0,
  "views_updated": 0,
  "components_added": 0,
  "components_updated": 0,
  "components_removed": 0,
  "memory_entries_added": 0,
  "selectors_verified": 0,
  "selectors_rejected": 0,
  "coverage_before": { "t1": 0, "t2": 0, "t3": 0 },
  "coverage_after":  { "t1": 0, "t2": 0, "t3": 0 },
  "validate": "pass | fail",
  "lint": "pass | fail",
  "notes": "one or two sentences on anything a human should look at"
}
```
