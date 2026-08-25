---
"@sightmap/sightmap": patch
---

Authoring skill: fix the "Further reading" pointers. They referenced `go/README.md` (not shipped with the installed skill) and `docs/reference.md` (which does not exist). The skill is self-contained — coverage model, outer loop, tool surface, lint rules, quality checklist, and the component model are all in it — and now points to the published docs for the normative spec instead of dead repo-relative paths.
