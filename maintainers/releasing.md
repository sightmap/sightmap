# Releasing

This repo ships several independently-versioned things. They release differently.

## 1. The spec

The spec version is the integer in the `version:` field of a sightmap file. Today that's `1`.

A "release of the spec" happens when:

- A new major version is cut (`v1` → `v2`). Rare, driven by an accepted SEP.
- An existing major version gets a meaningful change (new optional field, tightened schema, clarification that could affect behavior). Not a version bump, but still worth announcing.

### Cutting a new major version (the rare case)

Prerequisites:

- [ ] Accepted SEP(s) defining the changes
- [ ] `spec/v<N+1>/` directory exists with its own `schema.md`, `sightmap.schema.json`, and `examples/`
- [ ] At least one implementation has a PR open supporting the new version
- [ ] Announcement drafted in Discussions (don't publish yet)

Release steps:

1. Open a "Release spec v<N+1>" PR that updates `spec/README.md` to point to the new version as "latest", updates the sites to describe it, and tags the old version as "previous" per [`../spec/VERSIONING.md`](../spec/VERSIONING.md).
2. Once merged, create a signed git tag `spec-v<N+1>` on the merge commit.
3. Publish a GitHub Release with the tag; write the release notes from the SEP(s) and the diff.
4. Publish the Discussions announcement.
5. Notify implementation maintainers (Subtext internal, plus any community ports we know about) with a one-sentence heads-up and a link to the Release.
6. Watch for fallout. Pin an issue for migration questions.

### Meaningful change within a major version

No spec tag. Do:

- [ ] Update `spec/v<N>/schema.md` and `sightmap.schema.json` in one PR
- [ ] Update affected examples and conformance fixtures
- [ ] If visible on a site, update the copy in the same PR
- [ ] Post a short Discussion announcement with label `announcement`

## 2. The CLI / library (`@sightmap/sightmap`)

The Go implementation releases on a pushed semver tag. The pipeline lives in [`.github/workflows/release.yml`](../.github/workflows/release.yml):

1. Bump versions as needed and land the change on `main`.
2. Push a semver tag, e.g. `v0.2.0`.
3. goreleaser (config in [`go/.goreleaser.yml`](../go/.goreleaser.yml)) builds cross-platform binaries and publishes a GitHub Release.
4. The `@sightmap/sightmap` npm wrapper is published; its `postinstall` downloads the matching binary from that Release.

Requires one repo secret: `NPM_TOKEN` (npm automation token with publish rights to the `@sightmap` scope). `GITHUB_TOKEN` is provided automatically.

Before tagging: `cd go && go test ./...` is green, and the wrapper version will be synced to the tag by CI.

## 3. The websites

Both sites auto-deploy from `main`: `docs/` via the Mintlify GitHub app (custom domain configured in the Mintlify dashboard), `web/` via Netlify from its subdirectory with its own `netlify.toml`. Every merged PR that touches a site is effectively a release.

Checks before merging anything that affects a site:

- [ ] Preview looks right (Netlify posts a preview URL on `web/` PRs; for `docs/`, run `mint dev` locally and CI runs `mint validate` + `mint broken-links`)
- [ ] `web/` only: no regressions on the password gate until launch (see below)
- [ ] No dev-only env vars accidentally shipped

### Pre-launch: the marketing-site password gate

Until we flip `web/` public, the password gate stays on. It is not a security boundary — it keeps the site out of search indexes and casual browsing while we finalize content. When ready to launch:

1. Open a PR that removes `PasswordGate` from `web/src/App.tsx`, deletes the component, and removes any references from `web/.sightmap/`.
2. Same PR: update `web/public/robots.txt` to allow indexing.
3. Same PR: remove any "internal / do not share" caveats from the copy.
4. Merge on the day of launch. Not before.
5. Verify the gate is gone in prod. Announce per the launch plan.

## 4. Breaking glass

If we need to pull a site down: for `web/`, Netlify dashboard → site → Deploys → Publish a previous deploy; for `docs/`, revert the commit on `main` (the Mintlify GitHub app redeploys) or unpublish from the Mintlify dashboard. If a spec file at the raw GitHub URL is wrong, a fast-follow PR is the right answer — we can't "unpublish" a commit on `main`. To yank a bad CLI release, deprecate the npm version and delete/mark the GitHub Release, then cut a fixed patch.
