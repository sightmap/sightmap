---
"@sightmap/sightmap": minor
---

`render.Props` and `render.PropEquals` export the rules `Format` uses for a matched node's properties: which properties it shows (the extracted ones, plus the AX value as `value` unless privacy withholds it), and which name or StaticText child it leaves out because a property already shows it. Renderers that keep their own tree and only reuse the matcher can call them so their output agrees with `Format`.
