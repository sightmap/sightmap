---
"@sightmap/sightmap": minor
---

Match results carry the matched definition's SEP-0020 ids, so a consumer can record what matched by a stable identity instead of a name: `ID` (and `RefID` for a `$ref` expansion's root) on `match.ChainMatch` and `sightmap.ComponentMatch`, and `ID` on `RequestMatch` and `MessageMatch`. They are "" when the sightmap declares no ids.
