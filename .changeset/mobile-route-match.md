---
"@sightmap/sightmap": patch
---

View routes now match slashless page names, such as the screen or view-controller name a native app reports as its page URL (`Shop`, `App.ProductViewController`). A page name is rooted before matching and compared case-insensitively, so a view routed `/shop` matches `Shop`, `shop`, and `SHOP`. URL paths that start with `/` match exactly as before, case-sensitively.
