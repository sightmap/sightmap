---
"@sightmap/sightmap": patch
---

Live shadow-piercing query (`sel-probe`, `click`/`fill`, property extraction) no longer under-reports on pages with a form.

`__smDeepQueryAll` walked the DOM via instance properties (`node.children`, `el.matches`, `el.shadowRoot`). On an `HTMLFormElement`, a control whose `name`/`id` is `children` makes the form's named getter return that control instead of the element `HTMLCollection` (`[LegacyOverrideBuiltIns]`), so `for (const c of node.children)` threw "children is not iterable" on that form. The blanket `catch` then returned a **truncated** result — silently `0` matches for any element after the form in document order (observed on jetblue.com: `[data-fs-element="submit"]`, `a.rounded-button`, `[data-fs-element="points-toggle"]`, and others reported 0 live while `querySelectorAll` and the offline matcher saw them).

The traversal now goes through cached `Node.prototype`/`Element.prototype` accessors, which bypass any named-property shadowing, so form-heavy pages resolve correctly. Affects every live lookup: `sel-probe`, component-query `click`/`fill`/`hover`, and live property extraction.
