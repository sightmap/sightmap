---
"@sightmap/sightmap": patch
---

`validate` no longer reports "duplicate component name and selector" for one child name used under two different parents. Child names are scoped to their parent, but the check keyed on the bare name across a flattened scope, so two definitions (or two components in one view) that each contain, say, a `Label` matching the same selector were flagged. The check now keys on the component's address (parent chain plus name); a true duplicate under one parent is still an error.
