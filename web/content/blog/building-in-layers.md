---
title: 'Composing Sightmap and Sightkick: A Map, a Toolbox, and a Spec'
excerpt: 'Sightkick operates at the CUA layer, giving models a clean tool interface to your app. Let agents compose those tools into test suites and you get model intelligence during setup and maintenance, paired with sub-second, zero-token execution on every commit. Three layers: a map, a toolbox, and a spec.'
topic: 'research'
date: '2026-09-22'
author: 'Clint Ayres'
slug: 'building-in-layers'
image: '/blog/og/building-in-layers.png'
---

There are more ways to be successful in browser automation today than ever before. Computer-Using Agents (CUAs), WebMCP, and fast System 1 and System 2 models are expanding what's possible, opening up incredible avenues for dynamic web interaction and autonomous navigation.

Sightkick operates directly at that CUA layer, providing a clean, structured tool interface for models to interact with web apps. But rather than relying on live CUA reasoning to drive the browser on every build, we can now let agents compose those tools together, authoring and resolving test suites that execute imperatively, deterministically, and virtually for free in CI. You get model intelligence during setup and maintenance, paired with the sub-second speed and zero-token execution cost on every commit.

That shift in perspective is why we built **Sightmap** and **Sightkick**. Together, they establish a three-tiered architecture built for agent-defined, deterministic execution: a map, a toolbox, and a spec. Each layer is a checked-in artifact, referencing the layer below it strictly by name, and nothing more.

## Three Layers, Bottom to Top

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/pyramid.jpg" alt="An isometric diagram of a three-tier pyramid. Bottom tier, indigo, labeled Components / .sightmap, its surface a grid of small named rectangles. Middle tier, teal, labeled Tools / .sightkick, eight connected tool icons in a dotted loop. Top tier, amber, labeled Features / .feature, a single sheet of paper." />
</figure>

1. **Components (`.sightmap`): The Map**
   A Sightmap names every view, component, and request in your application (e.g., `MenuCard`, `ApplyPromoButton`, `ReviewTotals`). **This is the only place raw CSS selectors exist.** Tools and specs reference components strictly by name, never raw DOM selectors.

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

2. **Tools (`.sightkick`): The Toolbox**
   Sightkick operates at the CUA layer, using System 1/2 models against your Sightmap to turn component names into atomic, callable actions (`apply_promo`, `read_cart`, `place_order`). Running `sightkick build` validates the entire toolset, instantly failing if a tool targets an unmapped component.

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

3. **The Spec (`.feature` + Plan): The Execution Artifact**
   The high-level intent: a standard `.feature` file paired with a compiled execution plan. Every line of Gherkin maps directly to one tool call and an expected assertion, checked into source control as clean JSON.

   ```json
   {
     "gherkin": "And I apply the promo code \"BURRITO20\"",
     "tool": "apply_promo",
     "params": { "code": "BURRITO20" },
     "expect": { "value": { "contains": "$18.92" } }
   }
   ```

When UI code changes, you update **one line** in the Sightmap corpus. The tools, executable plans, and scenario specs remain completely untouched.

## Watch It Resolve

Below is a 13-step plan (`examples/burrito/plans/purchase.plan.json` from the sightkick repo) resolved and executed against our demo app, Burrito Co. First, the real screenshots, one frame per stage, each carrying the guidance breadcrumb that pointed there:

<div data-widget="sightkick-frames" data-figure="journey">
<img src="/blog/images/sightkick/tool-01-menu.png" alt="The Burrito Co. menu, five items with prices, at the start of the purchase journey." />
</div>

When you step through the trace, all three layers move in lockstep: the Gherkin intent triggers the deterministic tool call, which targets the mapped components.

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

This architecture unlocks two critical advantages:

- **Journeys are advisory guidance, not fragile controllers.** The original `purchase` journey suggested 14 steps, including inspecting item customizations. When resolving the plan, the agent recognized the inspection step wasn't required for this run and bypassed it cleanly without breaking execution.
- **State complexity is absorbed once.** Burrito Co.'s checkout wizard handles Delivery, Payment, and Review under a single URL. The Sightmap corpus exposes `CheckoutSteps.activeStep` as a property, allowing every mutating tool to guard against state automatically, removing the burden of URL-guessing from the runner entirely.

## Three Closed Loops Beat One Open Problem

<figure class="shot shot-wide">
<img src="/blog/images/building-in-layers/closed-loop.jpg" alt="Left: one glowing hexagon holding a small robot icon and a circular loop of arrows with a checkmark badge, labeled One Closed Loop. Right: seven of the same hexagons tessellated into a honeycomb, connected edge to edge, labeled Composed." />
</figure>

Asking an agent to solve browser testing as an open-ended task (map the page, click around, figure out if it worked) creates an unconstrained search space with no feedback loop.

Decomposing the problem into three isolated, deterministic loops makes failure local and legible:

- **Components (Map):** Verified by coverage metrics. Target is zero unmapped orphan elements.
- **Tools (Actions):** Verified by `sightkick build`. Validates all tool references against the component map and offers corrections on missing elements.
- **Specs (Intent):** Verified by dual-hash validation over scenario text and compiled tool manifests. Mismatches halt execution before running bad builds.

Each closed loop delivers an immediate pass/fail signal. Composing them yields a test suite that is lightning-fast in CI, yet completely authored and maintained by agents.

## Where to Start, and What's Next

Build from the ground up: map a view down to zero orphans, use Sightkick with a System 1/2 model at the CUA layer to expose its tools, and resolve feature specs against those tools. Each layer can be built and verified before the layer above it exists.

**A couple of things to keep in mind:**

- **Model dependency during setup:** Sightkick relies on System 1/2 models during the authoring and plan-resolution phase. Once a `.feature` file is resolved into a checked-in JSON plan, execution in CI is deterministic and requires no live model reasoning.
- **Navigation boundaries:** Tools are intentionally scoped to single documents due to current [WebMCP](https://webmachinelearning.github.io/webmcp/) cross-document response handling. This boundary keeps tools modular and reusable across distinct scenarios.

Ready to build agent-defined, deterministic specs?

- **Start mapping:** [github.com/sightmap/sightmap](https://github.com/sightmap/sightmap)
- **Build your toolbox:** [github.com/sightmap/sightkick](https://github.com/sightmap/sightkick)
