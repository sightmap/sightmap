---
"@sightmap/sightmap": patch
---

Authoring skill: document the component model. Add "Component hierarchy (`children:`)" and "Cross-view references (`$ref`)" sections covering nested components, parent-scoped selectors, the depth budget, and `$ref` attestation. The browser skill now spells out that component queries use a descendant combinator (no `>` child combinator) and that a multi-match is an ambiguity error, not a missing component.
