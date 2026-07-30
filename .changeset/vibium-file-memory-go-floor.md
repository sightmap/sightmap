---
"@sightmap/sightmap": minor
---

Go library: `Corpus.Memory` now carries file-level `memory` entries (the loader previously dropped them). Lower the module's `go` directive from 1.25.2 to 1.23, its actual dependency floor, so consumers aren't forced onto a newer toolchain than the code requires.
