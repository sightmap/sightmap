# @sightmap/sightmap

npm thin-wrapper for [sightmap](https://github.com/sightmap/sightmap) — the sightmap authoring toolkit. Ships the correct pre-built native binary for your platform as an optional dependency — no download step, no Go toolchain required.

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

## How it works

The native binary is distributed as a set of per-platform packages
(`@sightmap/sightmap-darwin-arm64`, `@sightmap/sightmap-linux-x64`,
`@sightmap/sightmap-win32-x64`, …), each declaring its `os`/`cpu`. They are listed
as `optionalDependencies` of this package, so npm installs only the one matching
your machine. The `sightmap` launcher resolves that package and execs its binary.

This means installs are offline-friendly, lockfile-pinned, and integrity-checked
like any other dependency — there is no post-install download.

If you install with optional dependencies disabled (`--no-optional` /
`--omit=optional`), the binary won't be present and `sightmap` will explain how to
recover.
