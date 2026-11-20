---
"@sightmap/sightmap": minor
---

`sightmap browser` gains commands that read and check component state from the corpus, so an agent
verifies an app without `eval` scripts or `sleep`s: `get QUERY` prints a component's corpus
properties and text as JSON lines; `expect [QUERY] --prop k=v / --text / --count / --gone /
--focused / --url / --title` waits until every condition holds and on timeout prints what it saw;
`keys` sends a key sequence and typed text in one call; `run FILE` runs browser steps from a file
and stops at the first failing one with the page it was on. Actions (`navigate`, `click`, `fill`,
`keypress`, `keys`, `drag`) take `--settle` to wait until the DOM is quiet with no fetch in
flight; `eval` prints a hint naming the command that does what its script does. The
`sightmap-browser` skill leads with the observe, act, check loop.
