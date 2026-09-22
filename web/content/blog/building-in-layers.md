---
title: 'Building in layers: composing sightmap and sightkick'
excerpt: 'A sightmap names what your app is. A sightkick tool layer turns those names into callable actions. A feature file composes actions into a workflow. Each layer names only the one below it, and each one answers pass or fail on its own. Watch a real 13-step purchase resolve through all three, live.'
topic: 'research'
date: '2026-09-22'
author: 'Clint Ayres'
slug: 'building-in-layers'
image: '/blog/og/building-in-layers.png'
---

A `.feature` file has held up for ten years. It reads like a spec because it is one: `Given`, `When`, `Then`, in the language the business actually uses. What never held up is the layer underneath it: one hand-written step definition per line, a regex plus imperative browser code with selectors baked in. That glue is a second codebase, and it rots independently of the spec it exists to protect. You've debugged that suite. The `.feature` file was fine, and the failure surfaced three layers away from the sentence it was supposed to guard.

[Sightmap](/blog/sightmap) and [Sightkick](/blog/sightkick) are our answer to that glue. This post is about the shape the answer takes once both exist: three layers, each one a checked-in artifact, each one naming the layer below it and nothing more.

## Three layers, bottom to top

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/pyramid.jpg" alt="An isometric diagram of a three-tier pyramid. Bottom tier, indigo, labeled Components / .sightmap, its surface a grid of small named rectangles. Middle tier, teal, labeled Tools / .sightkick, eight connected tool icons in a dotted loop. Top tier, amber, labeled Features / .feature, a single sheet of paper." />
</figure>

Each layer below is real, taken from Burrito Co.'s checkout, and each one names only the layer directly underneath it.

1. **Components.** A [sightmap](/blog/sightmap) names every view, component, and request in your app, standing in for the raw DOM. Here's `ReviewTotals`, the order-totals block on the checkout Review step:

   ```yaml
   - name: ReviewTotals
     selector: '.review-totals'
     description: 'Order totals breakdown on the Review step'
     properties:
       - name: total
         extract: ReviewTotalsAmount.text
     children:
       - name: ReviewTotalsAmount
         selector: '.review-totals__row--total'
         properties:
           - name: text
             extract: text
   ```

2. **Tools.** A [sightkick](/blog/sightkick) tool layer turns those names into atomic, callable actions. Here's `apply_promo`, and note that every `query:` field names a corpus component, never a CSS selector:

   ```yaml
   - name: apply_promo
     description: Apply a promo code on the Review step and read the new total.
     ensure_view: Checkout
     params:
       - name: code
         type: string
         required: true
         description: The promo code. BURRITO20 is the only one that works.
     guard:
       absent:
         query: PromoField
     steps:
       - fill:
           query: PromoField
           value: '{{code}}'
       - click:
           query: ApplyPromoButton
       - wait_for:
           query: PromoAppliedLabel
     returns:
       description: 'The order total after the promo, e.g. "Total: $18.92".'
       value:
         query: ReviewTotals
         property: total
   ```

3. **A spec.** A `.feature` file plus a resolved plan, checked in as JSON. Every Gherkin line maps to one tool call and one expectation. Here's the step that calls `apply_promo`:

   ```json
   {
     "gherkin": "And I apply the promo code \"BURRITO20\"",
     "tool": "apply_promo",
     "params": { "code": "BURRITO20" },
     "expect": { "value": { "contains": "$18.92" } }
   }
   ```

Follow one name through all three layers. `ReviewTotals` is declared once, in the corpus. `apply_promo` addresses it by that name, in its `returns.value.query`, never by `.review-totals` directly. The plan step calls `apply_promo` and never mentions `ReviewTotals` at all; it doesn't need to, because the tool already resolved that name. Each layer references the layer below only by name, and nothing else. When `ReviewTotals`'s markup changes, one line of the corpus changes, and every tool and every plan that names it keeps working. A test written against `.review-totals` directly needs fixing at every call site that used it.

## Watch it resolve

Below is a real 13-step plan, `examples/burrito/plans/purchase.plan.json` from the sightkick repo, run against Burrito Co., the demo app from both posts above. First, the real screenshots, one frame per stage, each carrying the guidance breadcrumb that pointed there:

<div data-widget="sightkick-frames" data-figure="journey">
<img src="/blog/images/sightkick/tool-01-menu.png" alt="The Burrito Co. menu, five items with prices, at the start of the purchase journey." />
</div>

Now the same run from the other side. Step through it and all three layers move together: the Gherkin line, the tool call it resolved to, and the corpus components that tool addresses.

<div data-widget="feature-trace">
<pre>
Order two steak burritos with a promo code

  ✓ Given the menu lists five items
  ✓ When I open "Classic Burrito"
  ✓ And I customize "protein" as "steak"
  ✓ And I increase the quantity to 2
  ✓ And I add it to the cart
  ✓ Then the cart holds one line for "Classic Burrito" at "$21.90"
  ✓ When I check out
  ✓ And I enter the delivery address "123 Main St", "Denver", "CO", "80203"
  ✓ And I pay with card "4242 4242 4242 4242" expiring "09/26"
  ✓ And I apply the promo code "BURRITO20"
  ✓ Then the order total is "$18.92"
  ✓ When I place the order
  ✓ Then I get an order id
</pre>
</div>

Two things worth pulling out of that trace.

The plan runs 13 steps, but the `purchase` journey it was resolved from lists 14, one more, `read_item_customizations`, between opening the item and customizing it. A journey is advice compiled into guidance breadcrumbs; it grants no capability. The agent that resolved this plan chose not to take that step, and the plan still runs, because a journey was never in a position to stop it.

And Burrito Co.'s checkout is a three-step wizard, Delivery, Payment, Review, all on one route; the URL never changes between steps. `ensure_view: Checkout` can't tell them apart, so the wizard's position is a declared corpus property, `CheckoutSteps.activeStep`, and every mutating tool on that view guards and waits on it instead of the URL. That's a fact about the app that would otherwise get re-derived by every test that touches checkout. It's absorbed once, in the corpus, and every tool above it inherits it for free.

## Three closed loops beat one open problem

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/closed-loop.jpg" alt="Left: one glowing hexagon holding a small robot icon and a circular loop of arrows with a checkmark badge, labeled One Closed Loop. Right: seven of the same hexagons tessellated into a honeycomb, connected edge to edge, labeled Composed." />
</figure>

Give an agent the whole problem (map the app, write the tools, write the tests, keep them all in sync) and it flails, because nothing tells it when it's wrong until something downstream breaks in a way that doesn't point back at the cause. Give it one bounded loop at a time and it converges, because failure is legible and local:

- **Components:** coverage. Orphaned elements is a number; the target is zero.
- **Tools:** `sightkick build`. It fails on any reference the corpus doesn't have and hands back candidates.
- **A spec:** two hashes, one over the scenario text, one over the compiled tool manifest. A mismatch stops the run instead of reporting a misleading result.

Each one answers pass or fail with no human in the loop, scoped to exactly the layer it verifies. One closed system at a time, composed into the complex one.

## Where to start, and what isn't there yet

Build order is the layer order: map one view down to zero orphans, write the tools that view affords, then write the feature that uses them. Each step is buildable and checkable without the layer above it existing yet.

A couple of things worth being honest about. Resolving a `.feature` file into a plan isn't automated; an agent does it once, by hand, and there's no resolver yet. A journey's compiler is a pairwise walk over hand-authored pairs, not a search, so there's no inference and no planning. And a tool never crosses a navigation, because [WebMCP](https://webmachinelearning.github.io/webmcp/)'s cross-document tool responses are unspecified upstream. The protocol forced that granularity, and the granularity turned out to be the right one anyway: a tool that doesn't know which scenario is calling it is a tool that's reusable across all of them.

Start with the map: [github.com/sightmap/sightmap](https://github.com/sightmap/sightmap). Then the tool layer: [github.com/sightmap/sightkick](https://github.com/sightmap/sightkick). Read the [sightmap](/blog/sightmap) and [sightkick](/blog/sightkick) posts for the mechanics of each.
