# @sightmap/sightmap

npm thin-wrapper for [sightmap](https://github.com/sightmap/sightmap) — the sightmap authoring toolkit. Downloads the correct pre-built binary for your platform at install time. No Go toolchain required.

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

## Skip download

Set `SIGHTMAP_SKIP_DOWNLOAD=1` to bypass the binary download (e.g. in CI where you supply the binary yourself).
