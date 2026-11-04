---
sep: 0027
title: Routes match slashless page names, case-insensitively
author: Chandler Van De Water (@chandlervdw)
status: Draft
created: 2026-11-02
updated: 2026-11-02
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A page URL whose path has no leading `/` is a **page name**, such as the screen or view-controller name a native app reports in place of a URL. Route matching roots a page name before matching (`Shop` is matched as `/Shop`) and compares it case-insensitively, so a view routed `/shop` matches `Shop`, `shop`, and `SHOP`. A path that starts with `/` is a URL path and matches exactly as before, case-sensitively. No web URL has a slashless path, so every existing corpus matches web pages exactly as it did.

## Motivation

Native apps have screens but no URL. When an analytics or replay pipeline records a native session, it commonly reports the current screen's name as the page URL: a view-controller class name (`App.ProductViewController`), a navigation route name (`Shop`, `Cart`), or a screen title. These strings have no scheme and no leading slash.

[Route matching](../v1/schema.md#route-matching) anchors every route at `/`, so under the current rules no route can match such a page URL. Only global components apply, and every view-scoped component silently fails to match. Authors work around it by making every component global, which loses the view structure the corpus exists to describe.

Rooting the name fixes the anchor, but leaves a second mismatch. Route authors write routes in lowercase by convention (`/shop`, `/cart`), while page names follow the platform's identifier conventions: Swift and Kotlin types are PascalCase, and navigation libraries commonly name screens `Shop` or `ProductDetail`. A page name is an identifier, not a path below a host, and the same screen is often reported with different casing across platforms or SDK versions of one app. Matching it case-sensitively would force each corpus to mirror one platform's casing and break when another reports the same screen.

## Proposal

### Shape

No new fields. Existing `route` patterns gain matches against page names.

```yaml
version: 1
views:
  - name: Shop
    route: /shop
  - name: Product
    route: /*ProductViewController
```

### Match inputs and expected results

| Page URL | Path kind | Matches view |
|---|---|---|
| `Shop` | page name | Shop |
| `shop` | page name | Shop |
| `SHOP` | page name | Shop |
| `App.ProductViewController` | page name | Product |
| `App.PRODUCTVIEWCONTROLLER` | page name | Product |
| `/shop` | URL path | Shop |
| `/Shop` | URL path | none (case-sensitive) |
| `https://example.com/shop` | URL path | Shop |
| `https://example.com/SHOP` | URL path | none (case-sensitive) |

### Semantics

A page URL is a **page name** when it does not start with `/` and does not contain `://`. A page name is taken as reported, minus any query (`?`) and fragment (`#`). It is not parsed as a URL, so `:`, `%`, and spaces are ordinary characters in it: `Shop:Cart` is a page name, not a URL with scheme `shop`, and `50% off` is a page name, not a malformed escape.

Any other page URL is parsed as a URL, and its path is the URL path. A URL whose path is empty (a bare origin such as `https://example.com`) is the root, as before.

When matching a route against a page name, an SDK:

1. roots the path by prepending `/`, then normalizes trailing slashes as for any path;
2. matches the route exactly as it would a URL path, with `*`, `**`, and `:param` keeping their meaning, except that literal characters compare case-insensitively.

A route must start with `/`. A route that does not is an error (`route-not-rooted`), so a corpus never relies on a pattern being rooted implicitly. No existing corpus is affected: every route in the spec's examples and fixtures already starts with `/`.

Specificity is computed from the route alone and does not change. Requests are matched by the same rule, though request URLs are full URLs in practice and so always have URL paths.

### Conformance

- An SDK MUST match a page name as specified above: rooted, then case-insensitively.
- An SDK MUST continue to match a path that starts with `/` case-sensitively.
- An SDK MUST decide whether a page URL is a page name by the rule above, without parsing it as a URL, and before rooting it. A page name and the URL path it roots to (`Shop` and `/Shop`) are different inputs and may match different routes.
- Case folding MUST cover at least ASCII letters. An SDK MAY apply Unicode simple case folding beyond that.

## Alternatives considered

**Root page names but stay case-sensitive.** This makes page names matchable, and is the smallest change. It was ruled out because page-name casing is set by the platform, not the corpus author, so lowercase routes, the convention everywhere else in a corpus, would silently miss PascalCase screen names. Authors would need to know each platform's casing to write a working route.

**Make all route matching case-insensitive.** Simpler to state, but URL paths are case-sensitive below the host, and `url.query` keys are already specified as case-sensitive for that reason. Changing it would alter the URL set every existing web route matches.

**A per-view `case_insensitive` flag, or a synthetic URL scheme for native pages.** Both push the problem onto every author, or onto every capture pipeline, for an input whose shape already identifies it. A later SEP can still define a native URL scheme; a page name with a scheme is a URL, not a page name, and is unaffected by this one.

**Do nothing.** View-scoped components never apply on native pages, which is the problem this SEP exists to fix.

## Migration

A sightmap whose route lacks a leading `/` now fails validation; adding the slash is the fix. Otherwise no existing sightmap needs to change. Web page URLs always have a path starting with `/` (or empty), so they match exactly as before. SDKs that rejected or failed to match slashless paths now match them. The reference CLI implements this rule. The browser extension is unaffected, since a browser pathname always starts with `/`; conformance fixture `040-route-page-name` covers it.

## Open questions

- A page name that contains `://` is a URL by this rule. A screen name containing it would have to be reported with a scheme or leading `/` to avoid that; none is known to exist.


## References

- [Route matching](../v1/schema.md#route-matching)
- SEP-0024 (Draft, [#524](https://github.com/sightmap/sightmap/pull/524)), which identifies native views by a screen selector instead of a route, and notes a native URL scheme as complementary.
