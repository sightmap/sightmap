---
"@sightmap/sightmap": patch
---

Complete the "client commands warn before falling back to the default CDP port"
contract from the `browser start` loud-session change: `browser navigate`,
`browser eval`, `browser tabs list`, `browser tabs new`, `browser tabs close`,
and `browser tabs resize` now route through the same warning-emitting resolver
(`resolveCDPAddr`) as the other 21 client commands. Previously they resolved
their CDP address via the older `resolveAddr`/`sessionAddr` helpers, which fell
back to the default CDP port silently when no usable session file existed for
the corpus — so an operator (or an agent scanning stderr) got the foreign-corpus
signal for the sibling commands but not for these six, and `browser tabs new`
could silently open a tab on a foreign session's Chrome. The address each
command resolves is unchanged when a session file is present or `--addr` is
set; only the no-session-file fallback now prints a warning to stderr first.
