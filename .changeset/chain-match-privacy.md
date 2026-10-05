---
"@sightmap/sightmap": minor
---

`match.ChainMatch` carries the chain node's effective SEP-0009 `Privacy`, resolved as `Match` resolves `ComponentMatch.Privacy`, and `match.Withholds` is exported. A consumer that classifies streamed elements by ancestor chain can now withhold properties read from blocked or masked nodes without reimplementing privacy resolution.
