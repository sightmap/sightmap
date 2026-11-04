---
sep: 0027
title: Routes match slashless page names
author: Chandler Van De Water (@chandlervdw)
status: Draft
created: 2026-11-02
updated: 2026-11-04
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A page URL whose path has no leading `/` is a **page name**: the name an app gives the page it is showing, in place of a URL. Route matching roots a page name before matching (`Shop` is matched as `/Shop`), so a view routed `/Shop` matches `Shop`. Matching stays case-sensitive. A path that starts with `/` is a URL path and matches exactly as before. No web URL has a slashless path, so every existing corpus matches web pages exactly as it did.

## Motivation

Some capture pipelines record a name the app chose for the page, and have no URL to record. A native app calls the SDK's page-naming API with a string such as `Shop`, `Order Details` or `App.ProductViewController`, and the pipeline stores it as a page property. A consumer of that session, such as a replay or review tool, then asks the corpus which view the page is. The string has no scheme and no leading slash.

[Route matching](../v1/schema.md#route-matching) anchors every route at `/`, so under the current rules no route can match it. Only global components apply, and every view-scoped component silently fails to match. Authors work around it by making every component global, which loses the view structure the corpus exists to describe.

A page name exists only where the app names its pages. An app that never does has no page name, only whatever screen identifier the platform reports, which is not an identity an author can rely on. SEP-0024's `screen` selector, which identifies a view by an element in the tree, is the way to identify a native view with no help from the app. This SEP covers the case where the app supplies a name, and composes with it (see below).

## Proposal

### Shape

No new fields. Existing `route` patterns gain matches against page names.

```yaml
version: 1
views:
  - name: Shop
    route: /Shop
  - name: Product
    route: /*ProductViewController
```

### Match inputs and expected results

| Page URL | Path kind | Matches view |
|---|---|---|
| `Shop` | page name | Shop |
| `shop` | page name | none (case-sensitive) |
| `App.ProductViewController` | page name | Product |
| `/Shop` | URL path | Shop |
| `/shop` | URL path | none (case-sensitive) |
| `https://example.com/Shop` | URL path | Shop |

### Semantics

A page URL is a **page name** when it does not start with `/` and does not contain `://`. A page name is taken as reported, minus any query (`?`) and fragment (`#`). It is not parsed as a URL, so `:`, `%`, and spaces are ordinary characters in it: `Shop:Cart` is a page name, not a URL with scheme `shop`, and `50% off` is a page name, not a malformed escape.

Any other page URL is parsed as a URL, and its path is the URL path. A URL whose path is empty (a bare origin such as `https://example.com`) is the root, as before.

When matching a route against a page name, an SDK roots the path by prepending `/`, normalizes trailing slashes as for any path, and matches the route exactly as it would a URL path, with `*`, `**`, and `:param` keeping their meaning.

A route must start with `/`. A route that does not is an error (`route-not-rooted`), so a corpus never relies on a pattern being rooted implicitly. No existing corpus is affected: every route in the spec's examples and fixtures already starts with `/`.

Specificity is computed from the route alone and does not change. Requests are matched by the same rule, though request URLs are full URLs in practice and so always have URL paths.

### Relationship to SEP-0024

Both SEPs feed the same question: does a view hold? A declared `route` holds when a page URL or a page name is available and the route matches it. A declared `screen` holds when the selector matches the tree. A view that declares both must satisfy both, and SEP-0024's rule 1 (a view that declares `screen` beats one that does not) applies unchanged. Where SEP-0024 says a route-only view never holds "with no URL", it means no URL and no page name.

### Conformance

- An SDK MUST match a page name as specified above: rooted, then like any other path.
- An SDK MUST continue to match a path that starts with `/` as before.
- An SDK MUST decide whether a page URL is a page name by the rule above, without parsing it as a URL, and before rooting it. A page name and the URL path it roots to (`Shop` and `/Shop`) are different inputs but match the same routes.

## Alternatives considered

**Match page names case-insensitively.** It would let one lowercase route serve both a web `/shop` and a native `Shop`. Dropped: no corpus needs it today, an app's page names are its own strings so an author who controls the app can write the route to match, and it is the one change that contradicts "Matching is case-sensitive". A later SEP can add it, for example as an opt-in, if a cross-platform corpus needs it.

**Make all route matching case-insensitive.** Simpler to state, but URL paths are case-sensitive below the host, and `url.query` keys are already specified as case-sensitive for that reason. Changing it would alter the URL set every existing web route matches.

**A synthetic URL scheme for native pages.** This pushes the problem onto every capture pipeline for an input whose shape already identifies it. A later SEP can still define a native URL scheme; a page name with a scheme is a URL, not a page name, and is unaffected by this one.

**Close this SEP and use `screen` alone.** `screen` needs identifiers visible in the captured tree, which not every pipeline records, and it does not use the name an app already supplies. The two answer different inputs.

**Do nothing.** View-scoped components never apply to a named page, which is the problem this SEP exists to fix.

## Migration

A sightmap whose route lacks a leading `/` now fails validation; adding the slash is the fix. Otherwise no existing sightmap needs to change. Web page URLs always have a path starting with `/` (or empty), so they match exactly as before. SDKs that rejected or failed to match slashless paths now match them. The reference CLI implements this rule. The browser extension is unaffected, since a browser pathname always starts with `/`; conformance fixture `040-route-page-name` covers it.

## Open questions

- A page name that contains `://` is a URL by this rule. A page name containing it would have to be reported with a scheme or leading `/` to avoid that; none is known to exist.
- Should a cross-platform corpus be able to match a web `/shop` and a native `Shop` with one route? This SEP does not, since it means case-insensitive matching.

## References

- [Route matching](../v1/schema.md#route-matching)
- SEP-0024 (Draft, [#522](https://github.com/sightmap/sightmap/pull/522)), which identifies views by a screen selector instead of a route.
