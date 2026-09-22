---
title: 'Building in layers: composing sightmap and sightkick'
excerpt: "What does browser automation look like when you design it for autonomous agents from day one? Three layers: a map that names the app, a toolbox of atomic actions over those names, and a spec that composes them. Each layer references the one below it only by name, and each one answers pass or fail on its own."
topic: 'research'
date: '2026-09-22'
author: 'Clint Ayres'
slug: 'building-in-layers'
image: '/blog/og/building-in-layers.png'
---

What does browser automation look like when you design it for autonomous agents from day one?

Ask an agent to operate a modern web app and it hits a wall. The raw DOM is noisy, CSS selectors churn, and apps hide their state behind dynamic routing. Forcing a model to parse markup, infer business logic, locate elements, and execute all at once guarantees failure. The agent isn't failing for lack of intelligence. It's failing because we handed it the wrong primitives.

An agent-first approach means rethinking the interface between models and web apps. Instead of treating the browser as a chaotic canvas of DOM nodes to guess against, decompose the interaction into structured, machine-legible layers.

That shift is why we built [sightmap](/blog/sightmap) and [sightkick](/blog/sightkick). Together they form a three-tier architecture built for agentic execution: a map, a toolbox, and a spec. Each layer is a checked-in artifact that references the layer below it strictly by name, and nothing more.

## Three layers, bottom to top

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/pyramid.jpg" alt="An isometric diagram of a three-tier pyramid. Bottom tier, indigo, labeled Components / .sightmap, its surface a grid of small named rectangles. Middle tier, teal, labeled Tools / .sightkick, eight connected tool icons in a dotted loop. Top tier, amber, labeled Features / .feature, a single sheet of paper." />
</figure>

Each example below is real, taken from Burrito Co.'s checkout.

1. **Components (`.sightmap`), the agent's map.** Before an agent can act it needs a reliable map of the terrain. A [sightmap](/blog/sightmap) isolates the raw DOM by naming every view, component, and request in your app. **This is the only layer where CSS selectors exist.** The agent never sees CSS, only semantic component names. Here's `ReviewTotals`, the order-totals block on the checkout Review step:

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

2. **Tools (`.sightkick`), the agent's capabilities.** Rather than asking an agent to write browser manipulation code, [sightkick](/blog/sightkick) turns component names into atomic, callable tools. Every `query:` field references a corpus component by name, so `sightkick build` can validate the entire toolset up front: an agent can never invoke an action against a component that doesn't exist.

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

3. **The spec (`.feature` plus a plan), the intent.** At the top sits human-readable intent: a `.feature` file paired with a compiled execution plan. Every Gherkin line maps to a single tool call and an expectation, serialized as checked-in JSON.

   ```json
   {
     "gherkin": "And I apply the promo code \"BURRITO20\"",
     "tool": "apply_promo",
     "params": { "code": "BURRITO20" },
     "expect": { "value": { "contains": "$18.92" } }
   }
   ```

Structuring the app this way decouples UI changes from agent reasoning. Follow one name up the stack: `ReviewTotals` is declared once, in the corpus; `apply_promo` addresses it by that name and never by `.review-totals`; the plan step calls `apply_promo` and never mentions `ReviewTotals` at all, because the tool already resolved it. When a frontend engineer changes that markup, you update **one line** in the map. Every tool, every stored plan, and the agent's reasoning all stay intact.

## Watch it resolve

Below is a 13-step plan, `examples/burrito/plans/purchase.plan.json` from the sightkick repo, running against our demo app, Burrito Co. First, the real screenshots, one frame per stage, each carrying the guidance breadcrumb that pointed there:

<div data-widget="sightkick-frames" data-figure="journey">
<img src="/blog/images/sightkick/tool-01-menu.png" alt="The Burrito Co. menu, five items with prices, at the start of the purchase journey." />
</div>

Now the same run from the other side. Step through the trace and all three layers move in lockstep: the Gherkin intent triggers a deterministic tool call, which resolves to the mapped corpus components.

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

An agent resolved that plan once. Every run since is a Node script calling the tools directly: no model, no tokens, no sampling variance. Two behaviors in the trace are worth pulling out.

**Journeys are guidance, not execution scripts.** The `purchase` journey lists 14 steps, one more than the plan runs, including a `read_item_customizations` inspection between opening the item and customizing it. A journey compiles into advisory breadcrumbs attached to each tool result; it grants no capability and gates nothing. The resolved plan simply doesn't include that step, and it runs clean, because a journey was never in a position to stop it.

**State complexity is absorbed at the map layer.** Burrito Co.'s checkout wizard handles Delivery, Payment, and Review under a single URL, so `ensure_view: Checkout` can't tell them apart. Rather than making every caller re-derive where it sits in the form, the corpus exposes `CheckoutSteps.activeStep` as a declared property, and every mutating tool on that view guards and waits on it. The state-tracking burden moves off the agent and into the layer that already knows.

## Three closed loops beat one open problem

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/closed-loop.jpg" alt="Left: one glowing hexagon holding a small robot icon and a circular loop of arrows with a checkmark badge, labeled One Closed Loop. Right: seven of the same hexagons tessellated into a honeycomb, connected edge to edge, labeled Composed." />
</figure>

Prompting an agent to solve web automation as one open-ended task (map the page, work out the buttons, run the flow, verify the output) creates an unconstrained search space with no feedback. When it fails, nothing tells the agent where.

Layering breaks that into three isolated loops, each with an immediate, local pass/fail signal:

- **Components:** coverage. Unmapped interactive elements are counted as orphans, and the target is zero.
- **Tools:** `sightkick build`. It validates every tool reference against the map and hands back concrete candidates when one doesn't resolve.
- **Specs:** two hashes, one over the scenario text, one over the compiled tool manifest. A mismatch halts the run before it can report a misleading result.

Each loop gives the agent a bounded problem with a legible edge. Compose the small verifiable loops and the complex one becomes tractable.

## Where to start

Build from the ground up: map a single view until orphans hit zero, define the tools that view exposes, then write the feature spec that uses them. Each layer is buildable and checkable before the one above it exists.

Worth knowing before you adopt it:

- **Plan resolution.** Translating a `.feature` file into a JSON plan takes one agent pass today, by hand-off. There's no automated resolver yet.
- **Journey compilation.** The compiler walks hand-authored pairs and attaches guidance. There's no inference and no graph search behind it.
- **Navigation boundaries.** Tools never cross a navigation, because [WebMCP](https://webmachinelearning.github.io/webmcp/) leaves cross-document tool responses unspecified. The protocol forced that granularity, and it turned out to be the right one: a tool that doesn't know which scenario is calling it is reusable across all of them.

Map your application: [github.com/sightmap/sightmap](https://github.com/sightmap/sightmap). Build your toolset: [github.com/sightmap/sightkick](https://github.com/sightmap/sightkick). The [sightmap](/blog/sightmap) and [sightkick](/blog/sightkick) posts cover the mechanics of each.
