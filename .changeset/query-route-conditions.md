---
"@sightmap/sightmap": minor
---

Query conditions in routes (SEP-0025, Draft): a view or request `route` may end in `?KEY=VALUE&KEY=:name&KEY=*`. Each condition reads the first decoded value of its key: `KEY=VALUE` requires that value (a `*` inside it is a wildcard within the value), `KEY=:name` and `KEY=*` require the key to be present. Keys the route doesn't name are ignored, so a route without conditions matches exactly as before. For views, each condition adds the score of the path segment it mirrors (3, 2, 1), so `/profile?section=gift-cards` wins over `/profile` on that section and `/profile` keeps the rest. `ViewForURL`, `TiedViews`, `TagsForURL`, `RequestsForURL` and `MatchRoute` read the URL's query string; `validate` reports `route-query-invalid` for a malformed query part and compares conditions in any order for `route-conflict`. The browser overlay picks views the same way.
