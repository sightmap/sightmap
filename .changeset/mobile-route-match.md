---
"@sightmap/sightmap": patch
---

View routes now match Fullstory mobile pages, whose URL is a bare view-controller name such as `KeanuShop.ProductViewController` with no leading slash. A path without a leading slash is rooted before matching, so a view routed `/KeanuShop.ProductViewController` (or `/*ProductViewController`) now applies its components there. Web URLs match exactly as before.
