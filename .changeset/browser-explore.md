---
"@sightmap/sightmap": minor
---

Add `sightmap browser explore`: drive the live tab toward a goal one step at a
time. Each step observes the page as the annotated component tree, offers the
interactive nodes as a short list of named actions, asks a picker which one to
take and whether the goal is already met, acts, and waits for the page to
settle, all over one CDP connection. The default picker is Jev (TypeSafe,
`TYPESAFE_API_KEY`), which answers in about 200 ms, so a step costs a few
hundred milliseconds with no big-model call; `--picker anthropic` puts Claude in
the same seat and `--plan` asks it once to write the goal's spec. Typed values
come only from `--value` or `--spec`; `--done-when` gives a deterministic finish
check; `--avoid` keeps the loop off named controls. `--bench SUITE.json` runs a
goal suite and prints a results table. The suites under `go/explore/bench/`
reproduce the published numbers.
