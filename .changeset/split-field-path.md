---
"@sightmap/sightmap": minor
---

Add `sightmap.SplitFieldPath`, an exported function that splits a `RequestPropertyDef.Field` body-source path into its dot-separated segments with backslash-escape support (`\.` for a literal dot in a JSON key, `\\` for a literal backslash). Previously `walkJSONPath` split on a plain `strings.Split(path, ".")`, so a JSON object key that itself contained a literal dot could never be addressed — there was no way to say "the key is literally `a.b`" versus "descend into `a` then `b`". `walkJSONPath` now uses `SplitFieldPath` internally, so existing unescaped paths behave identically and escaped ones now resolve correctly. External tools that need the same path-splitting semantics (for example, compiling a sightmap corpus into a target schema whose own path field documents the identical escaping) can call `SplitFieldPath` directly instead of re-deriving the parser.
