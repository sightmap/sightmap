# Sightmap tooling evals

A transcript-derived eval suite for the `sightmap` CLI's **agent-facing UX**:
does the tool, at each moment where a real authoring session went wrong, say
something that gets the author unstuck?

Unit tests in `go/` verify that the code does what the code intends. These
evals verify something different: that the tool's *behavior at failure points
observed in the field* is diagnosable. Every case here is chopped out of a real
end-to-end authoring transcript — the fixture reproduces the exact `.sightmap/`
state (or CLI invocation) from that moment, and the expectations encode what
the tool **should** have said.

- `transcripts/` — raw annotated transcripts. The initial dataset is
  [`2026-07-29-whichbars.md`](transcripts/2026-07-29-whichbars.md), a ~1h
  session in which an agent built a corpus for whichbars.com and, despite
  reaching "0 orphaned T3 ✓" on all four pages, committed a corpus with zero
  views because it never discovered the `views:` list structure.
- `ANALYSIS.md` — the failure-mode breakdown of that transcript, with root
  causes (file:line) and the recommended PR sequence.
- `cases/` — one directory per failure moment (plus `ok-*` regression guards).
- `run.sh` — build + run everything.

## Running

```bash
evals/run.sh                     # builds go/cmd/sightmap, runs all cases
evals/run.sh --only fm04-help-exits-nonzero --verbose
SIGHTMAP_BIN=/path/to/sightmap evals/run.sh   # test a prebuilt binary
```

The runner lives at `go/cmd/sightmap-evals`. Each case's fixture is copied to
a temp dir and executed there with an isolated `$HOME`, so runs never touch
your real `~/.sightmap` or the repo tree.

## Outcome semantics

| Result | Meaning | Exit |
|---|---|---|
| `pass` | expectations hold, case not marked `known_failing` | 0 |
| `known` | expectations fail, case marked `known_failing: true` — documented desired behavior awaiting its `recommended_fix` | 0 |
| `FIXED` | a `known_failing` case now passes — flip the flag to `false` in its `case.yaml` (usually in the PR that fixed it) | 1 |
| `FAIL` | a case not marked `known_failing` fails — a regression | 1 |
| `skip` | `requires:` not met on this machine | 0 |

So the suite is green in CI today, turns red if a fix regresses, and turns red
in the *other* direction when a PR fixes something without claiming it — the
dataset stays honest in both directions. The intended lifecycle: each PR in
`ANALYSIS.md`'s sequence flips its cases from `known` to `pass`.

## Case format

```yaml
id: fm05-session-file-red-herring
title: one-line statement of the failure
failure_mode: FM5              # groups cases; FM numbering lives in ANALYSIS.md
source:
  transcript: evals/transcripts/2026-07-29-whichbars.md
  moment: >
    What happened in the transcript, quoting the observed output.
requires: []                   # any of: linux/darwin/windows, chrome, manual
run:                           # optional — docs-only cases omit it
  args: [browser, status]      # sightmap CLI arguments
  fixture: fixture             # dir copied to a temp cwd for the run
  path_prepend: fixture/bin    # optional: prepended to PATH (fake binaries)
  timeout_seconds: 45          # default 60
expect:
  exit: zero                   # zero | nonzero | omitted (any)
  output:                      # regexes (Go syntax) on combined stdout+stderr
    - must_match: '(?i)unrecognized'
      why: what the assertion is protecting
    - must_not_match: 'assigned the same port'
      why: the red herring this case exists to kill
  files:                       # paths that must exist in the cwd after the run
    - exists: .sightmap
  docs:                        # checks on repo files (doc-drift cases)
    - file: skills/sightmap-authoring/SKILL.md
      must_match: '(?s)views:\s*\n\s+- name:'
known_failing: true
recommended_fix: PR-4          # from the sequence in ANALYSIS.md
notes: root cause pointers, caveats
```

`requires: [manual]` marks cases that need a live browser session against a
real page; the runner always skips them, but they stay in the dataset with
their desired behavior encoded.

## Adding cases from a new transcript

1. Drop the raw transcript into `transcripts/` (date-prefixed), and annotate
   the failure moments with `[→ FMnn]` tags plus a moment-index table.
2. For each distinct failure, reproduce the minimal `.sightmap/` state in a
   new `cases/<fmNN-slug>/fixture/` and confirm the *observed* (bad) output by
   running the case with `--only <id> --verbose`.
3. Write expectations for the **desired** output, not the current one. A good
   case usually has one `must_match` (what the tool should have said) and one
   `must_not_match` (the misleading thing it did say).
4. Mark it `known_failing: true` with a `recommended_fix`, and add the failure
   mode to `ANALYSIS.md` if it's new.
5. When behavior is fixed, flip `known_failing: false` in the same PR — the
   runner's `FIXED` result will remind you if you forget.

Keep fixtures faithful to the transcript (same filenames, same YAML shape,
same session-file bytes) — the point of the dataset is that these are states
real sessions actually reached, not synthetic ones.
