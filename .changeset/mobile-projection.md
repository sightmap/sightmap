---
"@sightmap/sightmap": minor
---

New Go package `projection`: the rules that reduce a native mobile component tree to the nodes a person or agent can perceive or operate (controls, nodes with their own text, nodes with an app id, named containers), so the iOS Simulator's tree (`sightmap device`) and a captured session's tree (lidar) can publish the same tree for a screen. Library only for now; nothing calls it yet.
