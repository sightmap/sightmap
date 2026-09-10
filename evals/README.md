# Browser evaluation foundation

This draft provides **observation-capture smoke measurements**, not a four-way
agent evaluation. The same predetermined DOM scripts run in both implemented
modes; observations do not choose the actions. Passing proves the fixture and
capture pipeline work, not that an agent could solve the task.

## Run

From `go/`, with an explicit installed Chrome executable:

```sh
SIGHTMAP_EVAL_CHROME='/path/to/chrome' \
SIGHTMAP_EVAL_OUT='../evals/results/local' \
go test ./evals -run TestMatrix -count=1 -v
```

No model key, browser service, npm install, user profile, or external website is
needed. The runner owns a temporary headless Chrome profile, ephemeral debugging
port, and loopback fixture server. It terminates that browser on exit. Ordinary
`go test ./...` skips browser measurement unless the variable is set, but runs
result-integrity tests. Runtime: Go from `go/go.mod`, Chrome, and existing Go
module dependencies. Browser startup failures fail the test before any trial;
started runs save partial results with `run_complete: false` on interruption.

Each of three tasks runs three times with fresh navigation before each mode.
Mode order alternates across repetitions; the tiny sample and shared browser
caches preclude performance conclusions. The fixture has no persistent storage.
Before actions the independent final-state predicate must be false, and after
actions it must be true. Every attempted cell is retained, including failure;
missing adapters have `status: not_run` and null measurements.

The checked-in example is under `results/smoke/`. It records the source revision,
runner SHA-256, dirty state/diff hash, fixture hashes, browser version, Go version,
platform, individual observations/actions, and all assertions. A dirty source
revision is not sufficient provenance on its own: retain the matching runner
and fixture bytes as this directory does. Reruns differ in timings, AX IDs and
browser metadata; compare invariants, not byte-for-byte goldens. Write new runs
to new directories to retain trial history.

## What the metrics mean

- `raw-ax`: the full JSON **result** of CDP `Accessibility.getFullAXTree`,
  unmodified, including hierarchy. It is not `snapshot` with a missing corpus.
- `sightmap`: the complete default `observe.Format` output, including view,
  guide, tree and coverage. The observer runs the real probe/AX/DOM pipeline.
- `observation_bytes`: sum of UTF-8 bytes returned before each scripted action.
  JSON and rendered text differ in serialization, filtering and content;
  their ratio does **not** isolate annotation benefit. This excludes task
  prompts, tool schemas, action results and provider message overhead.
- `capture_ms`: total successful observation acquisition/rendering time.
- `wall_ms`: observations plus scripted actions; excludes browser/server setup,
  reset and independent final assertions. Failed captures may add wall time but
  do not add successful capture time. Failed preconditions have zero timed work.
- `scripted_actions`: attempted DOM scripts, **not agent steps or tool calls**.
  Scripts use synthetic events and bypass target discovery and real input.
- `scripted_assertion_success`: fixture state only; the report task verifies the
  visible total, not an agent's textual answer.
- `model_tokens`: always null. No model or tokenizer is used.

Sightkick and Stagehand-facade cells are explicitly unavailable: their adapters
are not implemented here. No relative success, cost, speed or capability claim
about those tools follows from this smoke run.

## Extension protocol for actual model trials

Introduce a separate versioned result type with `execution_kind: model` rather
than silently changing the meaning of these scripted metrics. Each adapter must:

1. Reset the same app state, receive the same task prompt, and expose its declared
   observation/action surface. It must not receive the oracle, reference scripts,
   hidden DOM queries, or another mode's transcript.
2. Declare exact tool schemas, adapter/runtime versions, model/provider/version,
   prompts, generation settings, seed when supported, turn and time budgets,
   retries, cache policy, and task/corpus/tool-definition hashes.
3. Record every model request/response and tool result with timestamps. Count
   attempted agent turns, tool calls and actions separately; keep retries and
   failures in denominators. Obtain tokens from provider usage or a named,
   pinned tokenizer; never substitute bytes or estimates without labeling them.
4. Measure complete workflow wall time as well as capture time. Include prompts,
   tool schemas, observations and results in token accounting. Mark missing
   usage unknown, not zero. Apply identical budgets across modes.
5. Let the harness alone run final-state predicates. For read tasks independently
   check the submitted answer. Persist timeout, failure and unavailable states;
   never treat an unavailable trial as a pass. Randomize/counterbalance order,
   use multiple independent trials, report sample counts and uncertainty.

Future Sightkick adapters need pinned compiler/runtime and authored tool layers.
Future Stagehand adapters need a pinned facade/runtime and provider configuration.
Adding those adapters and actual model trials remains outstanding work.
