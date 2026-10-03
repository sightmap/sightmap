---
"@sightmap/sightmap": patch
---

render: keep the `[Component]` wrapper for a matched but invisible container — but only when a descendant survives. A sightmap-matched node whose container did not paint (e.g. `display:contents`, zero-size, or `visibility:hidden`) was being made transparent instead of kept, so its `[ComponentName]` annotation was dropped from the component tree and its visible children were promoted anonymously — while the `[Guide]` and T2 trace still attributed them to the matched component. The invisible-branch guard now matches its sibling transparency rules (`role="none"`, `IsIgnored`, empty `generic`), so a matched invisible node with visible descendants is kept and the tree agrees with coverage under `--trace`. A fully-invisible matched subtree (e.g. `display:none`, which probe.js propagates to all descendants) still renders nothing, so closed modals, collapsed menus, and inactive tabs do not appear as phantom `[Comp]` entries.
