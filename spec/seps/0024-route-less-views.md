---
sep: 0024
title: Views identified by a screen selector
author: Chandler Van De Water (@chandlervdw)
status: Draft
created: 2026-10-29
updated: 2026-10-29
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A view may declare `screen`, a selector that identifies it by an element present in the current tree, in place of or alongside `route`. A view must declare at least one of the two. A view **holds** when every identity it declares holds: its route matches the URL, and its screen selector matches some element of the tree. This gives native apps, which have screens but no URL, a way to say which view is showing, and lets a web corpus name a state that shares its URL with another (a modal, a wizard step). No existing view declares `screen`, so every existing corpus behaves exactly as before.

## Motivation

The spec defines a view as "a named screen in the app, identified by a URL route", requires `route` on every view, and selects the current view by matching the URL path. A native app on iOS or Android, including one built with React Native, has screens but no URL. SEP-0014 already lets a corpus declare native environments (`platform: ios`, `app_id`), so the spec admits native apps as deploy targets, but gives their views nothing to be identified by.

The reference CLI is gaining `sightmap device`, which snapshots a running app in the iOS Simulator as a sightmap component tree. Each node carries a role tag (`button`, `text`, `textfield`, `tab`, `other`, …) and the attributes `identifier` (the platform accessibility identifier, or a React Native `testID`; `testid` is accepted as an alias), `text` (the accessibility label) and `value`. Components match that tree with ordinary selectors (`button[identifier="login-submit"]`). Views cannot:

```yaml
views:
  - name: Login
    route: /login        # nothing produces or reads this
    components:
      - name: SubmitButton
        selector: 'button[identifier="login-submit"]'
  - name: Products
    route: /products     # nor this
```

Every route in such a corpus is a placeholder written to satisfy the schema. Since there is no URL to match, the CLI picks the view itself: the one whose own (non-global) components match the most nodes, unless the user passes `--view NAME`. That inference is a stopgap. It is a heuristic over content, it changes as data loads, it cannot separate two screens built from the same components, and nothing in the spec says how to compute it, so a second SDK would not agree with the first.

The web has a smaller version of the same gap. A checkout page that opens a payment sheet, or a settings page whose tabs do not change the path, has one URL and several states an author wants to describe as views, each with its own components and memory. Today the only option is a single view that carries every state's components.

## Proposal

### Shape

```yaml
# Before: placeholder routes, view inferred by the CLI
version: 1
environments:
  - name: ios-dev
    platform: ios
    app_id: com.example.shop
views:
  - name: Login
    route: /login
    components:
      - name: SubmitButton
        selector: 'button[identifier="login-submit"]'
  - name: Products
    route: /products

# After: each view says how to recognise it
version: 1
environments:
  - name: ios-dev
    platform: ios
    app_id: com.example.shop
views:
  - name: Login
    screen: '[identifier="login-screen"]'
    environments: [ios-dev]
    components:
      - name: SubmitButton
        selector: 'button[identifier="login-submit"]'
  - name: Products
    screen: ['[identifier="products-screen"]', 'heading[text="Products"]']
    environments: [ios-dev]
```

A web view may combine the two:

```yaml
views:
  - name: Checkout
    route: /checkout
  - name: CheckoutPaymentSheet
    route: /checkout
    screen: '[data-component="PaymentSheet"]'
    components:
      - name: CardNumber
        selector: 'input[name="card"]'
```

**JSON Schema diff.**

- `$defs.view`: remove `route` from `required`, leaving `required: [name]`; add `anyOf: [{required: [route]}, {required: [screen]}]`.
- `$defs.view`: add `screen: {$ref: "#/$defs/selector"}` (optional), described as "Selector that identifies this view by an element in the current tree. See SEP-0024."
- `$defs.view.properties.route.description`: note that a route is required only when `screen` is absent.
- `$defs.selector`: unchanged. `screen` takes the same string-or-list-of-alternatives shape components use.

No other `$defs` entry changes. Requests keep `route` as required.

### Semantics

**When a view holds.** A view holds when every identity it declares holds:

- a declared `route` holds when there is a URL and the route matches its path, by [Route matching](../v1/schema.md#route-matching);
- a declared `screen` holds when the selector, or any one of its alternatives, matches at least one element anywhere in the tree.

With no URL, as in a native snapshot, a view that declares only `route` never holds. With no tree, a view that declares `screen` never holds. A screen selector uses the component selector grammar and is matched against the same tree components are, flattened as described in [Selector semantics](../v1/schema.md#selector-semantics). It is evaluated against the raw tree, never against component matches, so choosing the view never depends on the components the view would bring in. The element it matches is not named by it; an author who wants that element named declares a component as well.

**Choosing the current view.** Among the views that hold, the current view is chosen by these rules in order, each consulted only to break a tie left by the one before:

1. A view that declares `screen` beats one that does not. Its identity was confirmed by evidence on screen, not by an address alone.
2. Route specificity, as in [View matching](../v1/schema.md#view-matching-most-specific-wins). A view with no route ranks below every view whose route held, including `/**`.
3. Screen specificity, by the ordering [SEP-0023](0023-most-specific-definition-wins.md) defines for component selectors, comparing the alternatives that matched: a view whose screen selector is strictly more specific than another's beats it.
4. First declared.

Rule 3 is sound for the same reason it is for components. If A's selector implies B's, every tree where A matches is a tree where B matches, so A's view is a refinement of B's: `[identifier="cart"] [identifier="coupon-sheet"]` describes a state of the cart, and should win over `[identifier="cart"]` whenever both hold. Most pairs of screen selectors are unordered (`[identifier="cart"]` and `[identifier="coupon-sheet"]`), and those fall to declaration order, the same fallback routes use. Should SEP-0023 not be accepted, rule 3 is dropped and declaration order decides directly; nothing else in this SEP depends on it.

Rule 1 keeps the web behaviour predictable: at `/checkout`, `CheckoutPaymentSheet` names the page while the sheet is open and `Checkout` names it otherwise, regardless of which is declared first or whose route scores higher.

**What "the view holds" now means elsewhere.** Every place the spec reads "the URL matches the view's route" now reads "the view holds". For route-only views with a URL these are the same statement.

- *View-scoped components* are matched when the view is the current view, as before. Choosing the view now needs the tree, so an SDK that cached compiled queries per URL must key them by the current view instead.
- *Memory* on a view applies whenever the view holds.
- *Tags* on views are a union across every view that holds, not only across those whose route matches. `CheckoutPaymentSheet` above carries `Checkout`'s tags as well as its own.
- *Signals.* A signal whose `ref` names a view is true while that view holds. Today's wording, "a view's route being active", is the special case for route-only views.
- *URL properties* are read only from a matched route; a view with no route, or no URL, produces none.
- *View-scoped requests* are still matched by their own `route` against the request URL, which native apps have like any other. They apply while their view is the current view, as before. A request still requires `route`.
- *`url:`* stays optional. It is a navigable address, so a view with only `screen` has no use for one, and tooling that navigates by `url:` (bulk capture, coverage reports) skips such views rather than failing.
- *Environments.* A view with only `screen` pairs naturally with native environments, but `screen` does not depend on platform, and `environments` remains declarative data rather than a matching input, as SEP-0014 states.

**Diagnostics.**

| Code | Severity | Meaning |
|---|---|---|
| `view-identity-missing` | error | A view declares neither `route` nor `screen`. (The schema already rejects it; the code names the failure for tools that report in diagnostics.) |
| `view-screen-invalid` | error | A `screen` selector, or one of its alternatives, does not parse. |
| `view-screen-shadowed` | warning | Two views declare the same `route` (or none) and identical `screen` selectors, so the later one can never be current. |

### Conformance

- MUST accept a view that declares `screen` without `route`, and MUST reject a view that declares neither.
- MUST treat a view as holding exactly when every identity it declares holds, as defined above, and MUST NOT treat a route-only view as holding when there is no URL.
- MUST choose the current view by the four rules above, in order.
- MUST evaluate `screen` against the raw tree, independent of component matching.
- MUST resolve view tags as a union across every view that holds, and MUST evaluate a view-ref signal as true exactly while its view holds.
- SHOULD surface memory from every view that holds.
- MAY continue to infer a view from component matches when no view holds, but MUST NOT do so when one does, and SHOULD say in its output that the view was inferred.

A conformance fixture covers a route-less native view, a web view refined by `screen` at a shared URL, screen ordering by SEP-0023 with a declaration-order fallback, and a route-only view that never holds without a URL.

## Alternatives considered

- **Do nothing: placeholder routes.** Corpora keep inventing paths no app produces. They mislead readers into thinking the app has URLs, they collide when two authors pick the same placeholder for different screens, and nothing reads them, so an SDK still has to guess the view.
- **Infer the view from component matches.** The `sightmap device` stopgap: pick the view whose own components match the most nodes. It is a heuristic over content, so the answer changes as lists load or empty out; it cannot separate screens that share components; and a tie-breaking rule two SDKs would compute identically would have to specify counting, scoping and weighting in more detail than this SEP's whole proposal. `screen` lets the author state what the heuristic guesses.
- **A synthetic URL scheme.** Native SDKs report a URL such as `app://shop/products/42`, and views keep `route`. This reuses route matching, specificity and URL properties unchanged. It requires the app to report a screen name: React Native navigation libraries know the current route and could, but a plain platform app has no such name to report, so the corpus would depend on instrumentation it cannot describe. It is complementary rather than competing: a later SEP can define such a scheme, and a view that declares both `route` and `screen` already composes with it.
- **Make `route` optional with no replacement.** A view with no route would need a matching rule, which means either inference (above) or one the spec has not written. It also makes an accidentally deleted route silently valid.
- **A component name instead of a selector.** `screen: LoginScreen`, naming a component whose presence identifies the view. It reads well, but makes view selection depend on component matching, which in turn depends on the chosen view's scoped components. Restricting it to globals would break that cycle; see Open questions.

## Migration

Nothing breaks. Every existing view declares `route`, none declares `screen`, so each one holds exactly when its route matched before, rule 1 never applies, and the current view, tags, memory and signals are unchanged. The schema change only relaxes `required`.

Native corpora written with placeholder routes keep validating and keep working with the CLI's inference. Once an SDK implements this SEP, an author drops the placeholder and adds a `screen` selector per view, usually on the identifier of the screen's root element. The `sightmap device` command switches from inference to the rules above whenever any view in the corpus declares `screen`, and keeps `--view NAME` as an override.

Tooling: `schema.md` (View table, View matching, Memory, Tags, Signal), `sightmap.schema.json`, an example under `spec/v1/examples/`, the docs reference page (regenerated), and the reference matcher's view selection, which moves from URL-keyed to view-keyed caching.

## Open questions

- **Component names as screens.** Should `screen` also accept a global component name, matched by that component's selector? It avoids repeating a selector the corpus already has, at the cost of a second lookup space for `screen`.
- **Android.** The device tree is defined today from the iOS accessibility tree. Android's resource IDs and content descriptions map onto `identifier` and `text`, but the role vocabulary is not yet fixed. That is a question for the snapshot format rather than for this SEP, which only needs some tree to match.
- **Visibility.** A native tree can include screens that are pushed but covered, such as the screen under a modal or a previous tab. Should a screen selector match only on-screen elements? This SEP matches the tree as captured and leaves pruning to the snapshot.
- **Rule 1 on the web.** Should a screen-bearing view with a less specific route (`/checkout/**`) beat a route-only view with a more specific one (`/checkout/payment`)? Rule 1 says yes. The alternative, route specificity first, is less surprising for existing route hierarchies but lets a broad route hide an open modal.

## References

- [View matching](../v1/schema.md#view-matching-most-specific-wins): the route specificity this extends.
- [SEP-0004](0004-component-tags.md): tag resolution as a union, extended here to views that hold.
- [SEP-0007](0007-signals.md): view-ref signals.
- [SEP-0014](0014-environments-and-origins.md): native environments, which give a route-less view its deploy target.
- [SEP-0023](0023-most-specific-definition-wins.md): the selector ordering rule 3 reuses.
