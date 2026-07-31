---
"@sightmap/sightmap": patch
---

Route matching: `**` is now a proper globstar. As a whole path segment it matches zero or more segments, so `/admin/**` matches `/admin` itself (as the spec always stated) as well as `/admin/users` and deeper, and `/a/**/b` matches `/a/b`. A `**` glued into a segment (e.g. `/foo**`) is treated as a regular single-segment `*` that does not cross a `/`. This fixes the previous behaviour where `/admin/**` failed to match `/admin` and `/messages**` failed to match `/messages`.
