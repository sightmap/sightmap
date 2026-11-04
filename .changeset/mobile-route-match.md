---
"@sightmap/sightmap": patch
---

View routes now match slashless page names, such as the screen or view-controller name a native app reports as its page URL (`Shop`, `App.ProductViewController`). A page name is rooted before matching, so a view routed `/Shop` matches `Shop`. Matching stays case-sensitive. A view or request route that does not start with `/` is now a `route-not-rooted` error.
