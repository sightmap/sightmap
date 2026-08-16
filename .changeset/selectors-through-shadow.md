---
"@sightmap/sightmap": minor
---

Make selectors behave consistently across shadow DOM. The captured component tree already flattens shadow roots (so offline matching, `sel-probe`'s offline count, and coverage pierce shadow), but every live-DOM operation re-found nodes with `document.querySelector`, which cannot cross shadow boundaries — so property extraction, interaction (click/fill/hover/value/scroll/wait-for), `bounds`, `sel-probe`'s live count, and `suggest`/`discover` were silently shadow-blind and disagreed with the corpus. A shared shadow-piercing resolver (`browser.DeepQueryJS`) now backs all of them, so live operations reach shadow-DOM content the same way matching does (property extraction on shadow-DOM leaves, interaction with shadow-DOM controls, and discovery of shadow-DOM links now work). `spec/v1/schema.md` formalizes the selector/tree model and its shadow-DOM semantics — a deliberate divergence from live `document.querySelector`, which does not cross shadow roots.
