---
"@sightmap/sightmap": patch
---

Fix: `coverage.hrefSuffix` now drops short all-digit record-id tails (`/orders/42`, `/blog/2023/06/15`) per its documented "numeric or hashed" contract. Previously `looksHashed` only caught 4+-digit runs, so 1–3 digit numeric ids were surfaced as per-instance `a[href$="/42"]` hints in `gap`/`explain` — correct on a single capture but broken on the next. Versioned routes containing a letter (`/products/v2`) are still kept.
