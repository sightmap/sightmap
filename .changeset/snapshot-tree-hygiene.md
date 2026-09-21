---
"@sightmap/sightmap": patch
---

Snapshot tree hygiene: exclude non-content nodes from the extracted component tree (`snapshot --tree-out`, `capture`, `--json`).

The tree builder now prunes two classes of node — together with their whole subtrees — before they reach output, so a captured `*.snap.tree.json` is app content only and is safe to use as an authoring/golden fixture without post-processing:

1. The CLI's own injected overlay (`#__sightmap-overlay`, `#__sightmap-tooltip`). It was visible and non-ignored, so it leaked into the tree JSON and enumerated as bogus candidates.
2. Document-metadata and non-rendered tags: `<head>`, `<script>`, `<style>`, `<meta>`, `<link>`, `<title>`, `<noscript>`, `<template>`, `<base>`.
