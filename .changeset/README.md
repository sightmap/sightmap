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

2. **PR merges to `main`.** The `changesets` workflow
   (`.github/workflows/changesets.yml`) notices the pending changesets and opens
   a "Version Packages" PR that:
   - Runs `changeset version` to bump `go/npm/package.json` and write
     `go/npm/CHANGELOG.md`.
   - Runs `scripts/sync-manifest-versions.mjs` to sync the new version into the
     plugin manifests (`.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`,
     `.codex-plugin/plugin.json`, `.cursor-plugin/plugin.json`).
   - Deletes the consumed `.changeset/*.md` files.

3. **You merge the Version Packages PR.** This is the version bump.

4. **You cut the release** by pushing the tag:

   ```sh
   git tag v<version> && git push origin v<version>
   ```

   That triggers `release.yml` (goreleaser + the npm publish). The tag step is
   **manual on purpose**: a tag pushed by a workflow (using `GITHUB_TOKEN`) does
   not trigger other workflows, so a human pushes it. Use the version that the
   Version Packages PR set in `go/npm/package.json`.

## Skipping the changeset

Pure infra / refactor / docs changes, or changes that don't affect the published
`@sightmap/sightmap` package, don't need a changeset. Open the PR without one.
