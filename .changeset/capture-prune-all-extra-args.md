---
"@sightmap/sightmap": patch
---

Fix `capture-prune --all` silently accepting extra positional view args. The usage guard rejected `--all` only when exactly one positional arg was supplied, so `capture-prune --all vA vB` slipped through and pruned every view in the corpus — including views the user never named — instead of erroring. The guard now enforces the documented `(<view> | --all)` mutual exclusion: `--all` is valid only with zero positionals, and exactly one `<view>` is required without `--all`.
