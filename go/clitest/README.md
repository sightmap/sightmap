# clitest — agent-facing CLI cases

A small, [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)-inspired
harness for the `sightmap` CLI's **behaviour at the moments a real session got
stuck**. Ordinary unit tests verify that the code does what the code intends;
these verify something narrower and complementary: that when the tool is used
wrong — a malformed corpus, a hand-written session file, a command that doesn't
exist — it says something that gets the author *unstuck* rather than something
silent, generic, or misleading.

These are plain integration tests. `go test ./clitest/` runs them; so does CI
(they live under `go/`, inside the existing `go test ./...` job). There is no
second harness binary and nothing to wire up.

## Running

```bash
go test ./clitest/                       # from go/
go test ./clitest/ -run TestCLICases/help-exits-nonzero -v
```

`TestMain` builds `./cmd/sightmap` once; each case runs it in an isolated temp
cwd with an isolated `$HOME`, so runs never touch your real `~/.sightmap`.

## Case format

One directory per case under `testdata/cases/<slug>/`, holding a `case.yaml`
and an optional `fixture/` whose contents become the run's working directory.
The directory name is the case id.

```yaml
title: "route: at the file root gets a generic unknown-field warning instead of teaching the views: structure"
issue: 108            # public GitHub issue number — the join key (see below). Omit if not yet filed.
known_bug: true       # xfail: expectations describe DESIRED, not-yet-shipped behaviour
requires: [linux]     # optional; any of: darwin linux windows chrome manual
run:                  # omit entirely for a repo-content-only case
  args: [validate]    # sightmap CLI arguments
  fixture: fixture    # case-relative dir copied to the run cwd
  path_prepend: bin   # optional: cwd-relative dir prepended to PATH (fake binaries)
  timeout_seconds: 45 # optional (default 60)
expect:
  exit: zero          # zero | nonzero | omitted (any)
  output:             # regexes (Go syntax) on combined stdout+stderr
    - must_match: 'views:'
      why: what this assertion protects (shown on failure)
    - must_not_match: 'flag: help requested'
  files:              # paths that must exist in the cwd after the run
    - exists: .sightmap
  repo:               # assertions on repo files (doc-drift cases)
    - file: skills/sightmap-authoring/SKILL.md
      must_match: '(?s)views:\s*\n\s+- name:'
```

`requires: [manual]` marks cases needing a live browser against a real page;
they are always skipped but stay in the dataset with their desired behaviour
encoded. `requires: [chrome]` runs only where a Chrome is on PATH (override
with `SIGHTMAP_TEST_CHROME=1`).

## The xfail ratchet

`known_bug: true` inverts the sense of the assertions, which keeps the dataset
honest in both directions:

| | expectations pass | expectations fail |
|---|---|---|
| `known_bug: false` | ✅ pass | ❌ **regression** |
| `known_bug: true` | ❌ **looks fixed — drop the flag** | ✅ pass (logged as a known bug) |

So a regression turns a passing case red, and a fix turns a `known_bug` case red
until someone claims it by removing the flag (and, usually, closing the issue in
the same PR). The known-bug cases *are* a prioritized, executable backlog.

## The join key: `issue:`

The single durable link out of a case is its **public GitHub issue number**.
That's the shared coordinate across all three tracking layers — the same issue a
local task references, and the same number a fix's PR closes. There is no
separate failure-mode / PR taxonomy: a case points at an issue, and that's it.

If a moment is worth a checked-in case it's worth a public issue (or it's an
ordinary unit test with no external ref). Decontextualized provenance — the
transcript or report a case came from, scrubbed so a later reader needs no prior
knowledge of the app it was captured against — belongs in that issue, not here.

## Intake ritual — turning a real session into cases

Real feedback arrives as a transcript, a field-notes file, or an outside
evaluation, often against a private or unreproducible app. Route each finding:

1. **Dedupe** against existing issues and the current cases — most findings are
   already tracked, and the point is reconciliation, not a fourth backlog.
2. **Decontextualize.** Reduce the finding to a minimal `.sightmap/` state (or a
   bare CLI invocation) that reproduces it with no app-specific context.
3. **Route it:**
   - behaviour gap or feature others will hit → a **public issue**;
   - pure code-correctness → an ordinary **Go unit test** in the owning package;
   - the tool was right, the words were wrong → a **doc/skill fix** (and maybe a
     `repo:` case here);
   - **a case here only** when it's an agent-facing diagnosability moment that
     reproduces as a fixture — silent, generic, or misleading output.
4. **Encode the desired output**, not the current one: typically one `must_match`
   (what the tool should have said) and one `must_not_match` (the misleading
   thing it did say). Mark it `known_bug: true` and link the `issue:`.
5. When the behaviour ships, drop `known_bug` in the same PR — the `FIXED`
   failure will remind you if you forget.
