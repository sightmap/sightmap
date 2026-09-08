---
title: 'Giving web apps a real callable surface (instead of making AI guess your DOM)'
excerpt: "Browser agents burn tens of thousands of tokens guessing which div is the submit button, then fall apart when someone renames a Tailwind class. Sightkick compiles a declared tool layer into typed WebMCP tools on document.modelContext, so the agent makes a function call and gets a clean result. Here's what we learned building it."
topic: 'research'
date: '2026-09-08'
author: 'Clint Ayres'
slug: 'sightkick'
draft: true
image: '/blog/og/sightkick.png'
---

Watching an autonomous AI agent try to use a modern web app is equal parts magical and painful.

If you've played with browser agents at all, you know the dance. The model takes a screenshot or grabs an enormous dump of the accessibility tree, burns tens of thousands of tokens trying to guess which `<div>` is the submit button, dispatches a click, and prays the single-page app doesn't hit a re-render race condition. It's slow, it burns through API credits, and the whole thing falls apart the moment someone changes a Tailwind class name.

A while back, we built [Sightmap](/blog/sightmap) to solve the first half of this mess by giving views, components, and network requests stable, semantic names. But naming things only gets you so far.

Even if an agent knows a button is called `ApplyPromoButton`, it still has to decide when to click it, what to pass to it, and whether the click actually did anything. Forcing an LLM to reason through DOM mechanics on every single step is the slowest, most expensive place to put that logic.

We wanted to see what happens if you move that execution layer directly into the browser.

That experiment became [Sightkick](https://github.com/sightmap/sightkick). You declare a `.sightkick/` folder of tools next to your sightmap, run a compiler to wire them up, and let the browser expose them directly via [WebMCP](https://webmachinelearning.github.io/webmcp/) on `document.modelContext`.

Instead of an agent flailing against raw markup, it makes a typed function call and gets back a clean result:

```js
await document.modelContext.executeTool({ name: 'apply_promo' }, { code: 'BURRITO20' })
// => { ok: true, value: "Total: $18.92", guidance: [ ... ] }
```

Here is what we learned building it, dogfooding it on our demo app ([Burrito Co.](https://github.com/sightmap/sightkick/tree/main/examples/burrito)), and watching where our initial assumptions failed.

## Make tools atomic, typed, and idempotent

When we started writing tool definitions, the temptation was to make them smart, letting a tool handle multi-step flows or cross route boundaries. That turned out to be a mistake.

Tools work best when they do exactly one thing at a single point in time. Here is what `apply_promo` looks like in `.sightkick/checkout.yaml`:

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
    description: 'The order total after the promo, e.g. "Total: $9.46".'
    value:
      query: ReviewTotals
      property: total
```

A few small details here save hours of debugging:

- **Names instead of CSS:** `PromoField` and `ApplyPromoButton` aren't CSS selectors. They are semantic names from our sightmap. If the front-end team updates the checkout markup tomorrow, we update the selector in one place in the sightmap, and every tool keeps working.
- **Handling retries with guards:** Autonomous agents get nervous when network latency spikes and love to retry calls. The `guard` directive checks the page state first. Once the promo is applied, Burrito Co. removes the input box and shows a confirmation badge. Because `PromoField` is gone, the guard catches it, skips the steps, and returns `skipped: true` with the current total rather than blowing up.

The guard is easier to look at than to describe. Here is the tool above running on a code that works, and the state change the guard keys off:

<div data-widget="sightkick-frames" data-figure="tool">
<img src="/blog/images/sightkick/tool-04-promo-before.png" alt="The Checkout Review step before the promo call, with an empty promo code field and a total of $23.65." />
</div>

## A tool is only as honest as what it waits on

This was an embarrassing lesson from our early test runs.

In our first pass at `apply_promo`, we set the `wait_for` step to watch `ReviewTotals`. But `ReviewTotals` is already rendered on the checkout screen whether your promo code works or not.

When the agent passed an invalid promo code, the tool filled the input, clicked apply, checked if `ReviewTotals` was on the screen (it was!), and happily returned `ok: true`, even though the order total hadn't budged and the promo was rejected. The tool was lying.

We fixed it by creating a dedicated component in the sightmap (`PromoAppliedLabel`) that only mounts when a discount successfully applies, and pointed `wait_for` at that. A bad code now fails out loud:

```json
{
  "message": "waitFor: timed out after 5000ms for query [\".checkout .promo-applied\"]",
  "ok": false
}
```

If your tool finishes a mutation and immediately returns without waiting for specific, unambiguous feedback, you've built a race condition machine.

`customize_item`, on the item page, is the shape to copy. Its `wait_for` watches for the option button to come back with a `selected` class, a state that only exists once the click has actually landed, and its `guard` checks that same thing up front:

<div data-widget="sightkick-frames" data-figure="customize">
<img src="/blog/images/sightkick/tool-02-item-before.png" alt="The Classic Burrito detail page with chicken selected in the PROTEIN group." />
</div>

## Don't drown the model with global tools

Burrito Co. has 30 tools across its entire flow. If you dump 30 tools into an agent's prompt on every page, decision quality plummets. The model spends context tokens wondering if it should call `place_order` while looking at the home menu.

Sightkick scopes tools dynamically by route. The browser runtime listens for navigation and uses `AbortController` to tear down old tools and register new ones right on `document.modelContext`:

- **On the Menu** (`/apps/burrito/`): the agent only sees 7 tools (`read_menu`, `open_item`, basic nav). Checkout actions don't exist.
- **On Checkout** (`/apps/burrito/checkout/`): menu actions disappear. Now `apply_promo`, `submit_payment_details`, and `place_order` light up.
- **On Confirmation** (`/apps/burrito/confirmation/`): all payment and ordering tools are unmounted, leaving `read_order_id` and `order_again` next to the global nav tools.

The agent doesn't have to guess what's legal; the page only offers what is actually callable right now.

## Breadcrumbs beat complex workflow engines

Once you have atomic tools, how does an agent know what sequence makes sense?

The standard engineering instinct is to build a heavy state machine or workflow orchestrator. We went with something much simpler: journeys.

A journey is just a plain list of tools and the human reason for each step:

```yaml
# abridged; the real purchase journey runs 14 steps
journeys:
  - name: purchase
    description: Order one customized item end to end, from the menu to a confirmed order id.
    steps:
      - tool: read_menu
        reason: see what's on offer and what it costs before choosing
      - tool: open_item
        reason: customizations only exist on an item's own detail page
      - tool: add_item_to_cart
        reason: commit the item — this lands you on the cart, not back on the menu
      - tool: read_cart
        reason: confirm what landed, with its line total, before paying for it
      - tool: go_to_checkout
        reason: only reachable from the cart, and only when the cart is non-empty
      - tool: apply_promo
        reason: BURRITO20 is only applyable on the Review step, before ordering
      - tool: place_order
        reason: submit from Review; a card ending 0000 is declined here, not earlier
      - tool: read_order_id
        reason: read the generated order id back as proof the order landed
```

Journeys don't run anything or restrict what an agent can do. Instead, the compiler walks this list and attaches a tiny hint to the response envelope of each step:

```json
{
  "guidance": [
    {
      "tool": "read_cart",
      "reason": "confirm what landed, with its line total, before paying for it",
      "when": "now"
    }
  ],
  "ok": true
}
```

That came back from `add_item_to_cart`, and it answers the question the agent would otherwise burn a snapshot on. Adding an item navigates: you land on the cart, not back on the menu. The envelope says so and names `read_cart` as the next call.

Without that breadcrumb, an agent clicks the button, pauses, takes another DOM snapshot to figure out where it ended up, and debates whether it needs to navigate. With the breadcrumb, it immediately calls `read_cart`. No wasted round trips.

Here is the whole purchase run, one frame per stage, each carrying the breadcrumb that pointed there:

<div data-widget="sightkick-frames" data-figure="journey">
<img src="/blog/images/sightkick/tool-01-menu.png" alt="The Burrito Co. menu, five items with prices, at the start of the purchase journey." />
</div>

## The accidental superpower: zero-token CI testing

The best thing about building a typed tool surface is what it does for testing.

End-to-end browser tests are notoriously brittle. But because our tools already handle selectors, waits, and state checks, we realized we had a complete test harness.

We write scenarios in plain Gherkin:

```gherkin
Feature: Order a burrito
  Scenario: Order two steak burritos with a promo code
    Given the menu lists five items
    When I open "Classic Burrito"
    And I customize "protein" as "steak"
    And I increase the quantity to 2
    And I add it to the cart
    Then the cart holds one line for "Classic Burrito" at "$21.90"
    When I check out
    And I enter the delivery address "123 Main St", "Denver", "CO", "80203"
    And I pay with card "4242 4242 4242 4242" expiring "09/26"
    And I apply the promo code "BURRITO20"
    Then the order total is "$18.92"
    When I place the order
    Then I get an order id
```

An agent translates this into a static plan (`purchase.plan.json`) just once.

Every run after that runs via a tiny Node script in CI. It hits the browser, calls the tools directly, and runs assertions. It uses zero LLM tokens and makes zero model API calls.

To keep things honest, the runner checks two hashes before executing: one for the feature file and one for the compiled tool manifest. If an engineer changes a tool definition or breaks an extractor, the hash check halts the build right away instead of running stale plans:

```
$ node scripts/run-plan.mjs examples/burrito/plans/purchase.plan.json
✗ examples/burrito's compiled manifest has changed since this plan was stamped — re-plan (or pass --stale-ok).
```

## Catching the bug humans click past

Running this deterministic plan caught a legitimate bug in Burrito Co. that we completely missed when clicking around manually.

On the Review screen, `apply_promo` fired, the coupon took, and the total dropped to $18.92. We called `place_order`, the app transitioned to the confirmation screen, and a green success banner popped up.

When humans test that flow, we see the success screen, assume it worked, and close the tab.

But our stored plan asserted on the final receipt. The confirmation screen read **Total Charged: $23.65**, the last frame of the journey above. The client UI had applied the coupon visually, but the backend checkout endpoint silently dropped the discount before charging the card. The runner flagged it immediately.

## Where to poke around

Treating web pages like computer vision puzzles for language models is a brute-force band-aid. If we want autonomous agents that don't flake out or run up massive bills, applications need clear, callable interfaces.

The compiler, the runtime, and the entire Burrito Co. test suite are open source at [github.com/sightmap/sightkick](https://github.com/sightmap/sightkick). Take a look, run the plan runner, and let us know what breaks.
