# docs

Source for [docs.sightmap.org](https://docs.sightmap.org), the documentation site for the [Sightmap](https://sightmap.org) open spec. Part of the [`sightmap/sightmap`](https://github.com/sightmap/sightmap) monorepo.

## Local development

```bash
pnpm install
pnpm dev      # http://localhost:4321
pnpm build    # production build to dist/
pnpm preview  # serve the production build
```

Built with [Astro Starlight](https://starlight.astro.build/). Content is hand-authored MDX under `src/content/docs/`.

The **Schema reference** page is generated from the canonical `spec/v1/schema.md`
by `scripts/sync-spec.mjs` (run automatically before `dev` and `build`); it is
gitignored, not hand-edited. Edit the canonical file under `spec/` instead.

## Contributing

Sign-off (DCO) is required on commits. See the [contributor guide](https://github.com/sightmap/sightmap/blob/main/CONTRIBUTING.md) and [Code of Conduct](https://github.com/sightmap/.github/blob/main/CODE_OF_CONDUCT.md).

## License

MIT — see the [repository LICENSE](https://github.com/sightmap/sightmap/blob/main/LICENSE).
