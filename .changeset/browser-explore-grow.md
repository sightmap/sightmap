---
"@sightmap/sightmap": minor
---

`sightmap browser explore --grow` builds the corpus while it explores. On every
page visited, unmapped interactive controls are grouped by their nearest stable
ancestor selector, classified by Jev (buttons, links, inputs, select, nav,
cards, or noise), named from a template, verified against the offline matcher,
and appended to the view's YAML with `label` (and `href`) properties. Pages with
no view get one named from their route pattern; a selector seen on a second view
is promoted to a global component. Each write is followed by a corpus reload and
validation and rolled back if the corpus stops loading. Starting from an empty
`.sightmap/`, three passes over the books.toscrape.com suite reach full
coverage of the visited pages with `validate` and `lint` clean.
