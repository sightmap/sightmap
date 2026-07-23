# @sightmap/sightmap

npm thin-wrapper for [sightmap](https://github.com/sightmap/sightmap) — the sightmap authoring toolkit. Downloads the correct pre-built binary for your platform on first run. No Go toolchain required.

## Usage

```sh
# Run without installing (one-off)
npx @sightmap/sightmap snapshot

# Install globally
npm install -g @sightmap/sightmap
sightmap snapshot

# Go users (builds from source)
go install github.com/sightmap/sightmap/go/cmd/sightmap@latest
```

The first `sightmap` invocation downloads the matching native binary from the GitHub release and caches it per-user (`~/.cache/sightmap/<version>/`, or `%LOCALAPPDATA%\sightmap\<version>\` on Windows). Later runs use the cache. Set `SIGHTMAP_CACHE_DIR` to change the location.

> The binary is fetched on first run rather than at install time because npm 11+ blocks install lifecycle scripts by default.

## Supplying your own binary

Set `SIGHTMAP_BINARY=/path/to/sightmap` to skip the download entirely and run a binary you supply — useful in CI or air-gapped environments.
