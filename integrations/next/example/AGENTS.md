# AGENTS.md

How a coding agent should work in this app. Install the skills first:

```bash
npx @sightmap/sightkick skills install     # sightkick-authoring, sightkick-debug, sightmap-authoring, sightmap-browser
npx agent-browser skills get core          # agent-browser's own workflow (also: skills get webmcp-gen)
```

## Before touching UI code

Read `.sightmap/` for the view you are changing — component names, properties, and the
`memory:` notes are the contract every tool and plan depends on. Keep
`data-component="Name"` attributes stable; they are the selectors.

## After changing UI code

1. `npm run build && npm run start`
2. `npx sightmap browser start --detach --url http://localhost:3000/`
3. `npx sightmap snapshot --coverage --url <the view>` — fix the corpus until `0 orphaned T3 ✓`
4. `npm run sightmap:validate` — the tool layer must still compile
5. `npm run test:plans` — every plan must pass; if a plan's expectation is now wrong,
   update the plan and re-stamp it, never loosen the expectation to get green
6. Commit the `.sightmap/` and `.sightkick/` changes with the code change

## Adding a tool (human-in-the-loop)

Draft one tool per real user action on the view, following `sightkick-authoring`. Then
stop and ask which of the drafted tools matter — do not add every possible action. Journeys
are ranked by the product owner from real usage, not guessed.

## Do not

- Regenerate `.sightmap/` from source. `sightmap-next seed` is a one-time scaffold.
- Put CSS selectors in `.sightkick/`. Tools reference corpus component names only.
- Edit `public/.well-known/*`, `public/sightkick-runtime.js`, or `webmcp.init.js` by hand;
  `sightmap-next build` writes them.
