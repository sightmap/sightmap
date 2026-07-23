# docs

Source for [docs.sightmap.org](https://docs.sightmap.org), the documentation site for the [Sightmap](https://sightmap.org) open spec. Part of the [`sightmap/sightmap`](https://github.com/sightmap/sightmap) monorepo.

Built with [Mintlify](https://mintlify.com). Content is MDX under this directory; `docs.json` defines navigation, theme, and redirects.

## Local development

```bash
npm i -g mint
mint dev            # preview at http://localhost:3000
```

Quality checks:

```bash
mint validate       # strict build validation
mint broken-links   # internal link check
```

## The generated schema page

The **Schema reference** page (`reference/schema.md`) is generated from the
canonical `spec/v1/schema.md` and checked in — Mintlify has no build step.
Never edit it directly; edit the spec, then regenerate:

```bash
node scripts/sync-spec.mjs
```

CI fails when the generated page drifts from the spec.

## Deployment

The Mintlify GitHub app deploys pushes to `main` automatically; there is no
build pipeline in this repo. The `docs.sightmap.org` custom domain is
configured in the Mintlify dashboard.

## Contributing

Sign-off (DCO) is required on commits. See the [contributor guide](https://github.com/sightmap/sightmap/blob/main/CONTRIBUTING.md) and [Code of Conduct](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md).

## License

MIT — see the [repository LICENSE](https://github.com/sightmap/sightmap/blob/main/LICENSE).
