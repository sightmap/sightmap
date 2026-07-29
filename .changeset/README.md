# Changesets

This directory holds [changeset](https://github.com/changesets/changesets) files
— per-change descriptions that drive the version bump for the
`@sightmap/sightmap` npm package (the `go/npm` workspace).

## Workflow

1. **You open a PR.** If the change is user-facing for the published package
   (anything that should appear in the changelog or trigger a version bump), run:

   ```sh
   npm run changeset
   ```

   Pick `patch` / `minor` / `major`, write a short summary, and commit the
   resulting `.changeset/*.md` file with your PR.

2. **PR merges to `main`.** The `release` workflow
   (`.github/workflows/release.yml`) notices the pending changesets and opens
   a "Version Packages" PR that:
   - Runs `changeset version` to bump `go/npm/package.json` and write
     `go/npm/CHANGELOG.md`.
   - Runs `scripts/sync-manifest-versions.mjs` to sync the new version into the
     plugin manifests (`.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`,
     `.codex-plugin/plugin.json`, `.cursor-plugin/plugin.json`).
   - Deletes the consumed `.changeset/*.md` files.

3. **A maintainer merges the Version Packages PR.** That merge is itself a push
   to `main`, so the `release` workflow runs again. With no changesets left to
   consume, it tags the release, runs goreleaser (cross-platform binaries +
   GitHub release), and publishes the `@sightmap/sightmap` npm packages — all
   in the same run. No manual `git tag` step.

## Skipping the changeset

Pure infra / refactor / docs changes, or changes that don't affect the published
`@sightmap/sightmap` package, don't need a changeset. Open the PR without one.
