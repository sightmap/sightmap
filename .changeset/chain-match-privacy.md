---
"@sightmap/sightmap": minor
---

`match.ChainMatch` carries the chain node's effective SEP-0009 `Privacy`, resolved as `Match` resolves `ComponentMatch.Privacy`, and `match.Withholds` is exported. A consumer that classifies streamed elements by ancestor chain can now withhold properties read from blocked or masked nodes without reimplementing privacy resolution. A privacy-declaring definition whose selector uses `:has()` cannot be located on a chain, so it fails closed: a `block` or `mask` one applies to every chain node and an `unmask` one is never honored.
