---
title: 'Sightkick: the callable surface of your app'
excerpt: 'A sightmap names what is on the page. Sightkick compiles those names into WebMCP tools an agent calls through document.modelContext, so operating your app is a typed call with a structured result instead of a pile of DOM guesses.'
topic: 'research'
date: '2026-09-08'
author: 'Clint Ayres'
slug: 'sightkick'
draft: true
image: '/blog/og/sightkick.png'
---

A [sightmap](/blog/sightmap) gives every view, component, and request in your app a semantic name. Today we're open-sourcing the sequel: [sightkick](https://github.com/sightmap/sightkick), which compiles that corpus plus a hand-authored tool layer into WebMCP tools an agent calls directly.

Naming things solved half the problem. An agent that knows the button in front of it is `ApplyPromoButton` still has to decide to click it, decide what to click before and after it, and decide whether the click worked. That reasoning happens every run, and it happens in the most expensive place possible: the model's head, one DOM observation at a time.

Sightkick moves it into the app. You write a `.sightkick/` tool layer next to your `.sightmap/` corpus, `sightkick build` resolves every reference in it against the corpus, and the result is a set of named tools the agent finds on the page:

```js
await document.modelContext.executeTool({ name: 'apply_promo' }, { code: 'BURRITO20' })
// { ok: true, value: "Total: $18.92", guidance: [ ... ] }
```

Everything in this post is a real transcript from a build and run against Burrito Co., the same demo app from the sightmap post. It has a `.sightmap/` corpus, a seven-file `.sightkick/` tool layer, and 30 compiled tools — the whole thing is checked in at [`examples/burrito`](https://github.com/sightmap/sightkick/tree/main/examples/burrito).

## A tool is one atomic action

A tool is one thing you can do at a single point in time. No navigation crossing mid-tool. It bundles ordered `steps` (fill, click, wait_for) and an optional `returns` read, and it yields a structured result.

Here is `apply_promo`, from `.sightkick/checkout.yaml`:

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

Four things are worth pulling out of that.

`PromoField`, `ApplyPromoButton`, `PromoAppliedLabel`, and `ReviewTotals` are corpus component names, not CSS. The compiler resolves each one against the sightmap and fails the build if it can't. When the markup changes, one selector moves in the corpus and this tool keeps working.

`params` become the tool's input schema, and `{{code}}` interpolates into any query, value, or URL.

`guard` makes the tool idempotent. Once the promo lands, the app replaces the input with a confirmation line, so `PromoField` is gone, `absent` no longer holds, and a second call skips its steps instead of failing.

`wait_for` is how the tool knows it worked. A tool that ends in a mutation and returns immediately is lying about a race, so the last step blocks on the app's own visible feedback before the `returns` read runs.

A pure read has no steps at all. `read_cart`, from `.sightkick/cart.yaml`:

```yaml
- name: read_cart
  description: List the cart's line items (name + line total).
  ensure_view: Cart
  returns:
    description: The cart rows (item name + line total).
    list:
      rows: CartItem
      fields:
        item: itemName
        line_total: price
```

`rows` is a component query; every match becomes a row. Each field maps an output key to a property the corpus declares on that component. Declared is the operative word: if `price` isn't an extractor in the sightmap, the build fails here rather than returning empty strings at runtime.

The guard is easier to see than to describe. `customize_item` selects one option in one group on the open item:

<figure class="shot shot-wide">
<img src="/blog/images/sightkick/tool-02-item-before.png" alt="The Classic Burrito detail page before the call: the PROTEIN group has chicken selected, quantity 1, and the button reads Add 1 to Cart, $10.95." />
<figcaption>Before `customize_item(group: "protein", option: "steak")`. The default selection is chicken.</figcaption>
</figure>

<figure class="shot shot-wide">
<img src="/blog/images/sightkick/tool-03-item-after.png" alt="The same page after the call: steak is now the selected option in the PROTEIN group." />
<figcaption>After. One call clicked the option and waited for it to come back selected. Call it again and the guard matches, so the steps are skipped and the result carries `skipped: true`.</figcaption>
</figure>

## A journey is a map, not a runner

Tools are atomic on purpose, which leaves an obvious gap: what order do you call them in? Sightkick's answer is deliberately not a workflow engine. A journey is a hand-authored list of tool names with a reason for each, and the compiler walks it pairwise and attaches "consider this next" guidance to each tool's own result.

From `.sightkick/journeys.yaml`:

```yaml
journeys:
  - name: purchase
    description: Order one customized item end to end, from the menu to a confirmed order id.
    steps:
      - tool: read_menu
        reason: see what's on offer and what it costs before choosing
      - tool: open_item
        reason: customizations only exist on an item's own detail page
      - tool: read_item_customizations
        reason: see each group's current selection before changing anything
      - tool: customize_item
        reason: set a group whose default isn't what you want (it self-skips if it already is)
      - tool: increase_item_quantity
        reason: quantity is set here, on the item, not in the cart
      - tool: add_item_to_cart
        reason: commit the item — this lands you on the cart, not back on the menu
      - tool: read_cart
        reason: confirm what landed, with its line total, before paying for it
      - tool: go_to_checkout
        reason: only reachable from the cart, and only when the cart is non-empty
      - tool: submit_delivery_address
        reason: the Delivery step gates the wizard; this advances it to Payment
      - tool: submit_payment_details
        reason: pass a real 16-digit card — the pre-filled masked one is rejected
      - tool: apply_promo
        reason: BURRITO20 is only applyable on the Review step, before ordering
      - tool: read_order_total
        reason: read the discounted total while it still exists — it doesn't survive to the confirmation
      - tool: place_order
        reason: submit from Review; a card ending 0000 is declined here, not earlier
      - tool: read_order_id
        reason: read the generated order id back as proof the order landed
```

Burrito Co. has two more: `revise_cart` ("Change your mind in the cart, adjust quantities, drop a line, then check the total") and `diagnose_error` ("Land on a known error fixture and report which error fired and why").

The most likely misreading is that a journey grants a capability. It doesn't. A journey never navigates and never runs anything, and an agent that reads the manifest can compose any valid sequence with or without one. The compiler's logic here is a dumb adjacency walk over hand-authored pairs, with no inference and no graph search. It exists for a live agent that only sees one result at a time and would otherwise re-read the whole manifest to decide what to try next.

The interesting consequence is what happens to the compiled artifact. `sightkick build` consumes the journey list into per-tool guidance and discards the graph itself. The IR that comes out is the runtime's artifact: resolved locators, extractors, predicates, and nothing a runtime doesn't execute.

That firewall matters, because stored test plans hash `build`'s output as their staleness check (more on that below). Anything you add to the IR becomes something that can invalidate every frozen plan in the repo. A journey graph is advice for a plan-time reader, not behavior, so putting it in the IR would mean that reordering two hints, a change no runtime can observe, would stale every plan you have. The plan-time view (`sightkick outline` and `sightkick explain`) is instead a read-only projection over the same manifest `build` compiles, adding no IR field and changing no hash.

## registerTool, and why the tool set changes per page

The runtime is a small script on the page. It reads the IR and hands each tool to the browser's `document.modelContext`, the [WebMCP](https://webmachinelearning.github.io/webmcp/) surface an in-page agent discovers tools through. The whole interface it depends on:

```ts
export interface ModelContext extends EventTarget {
  registerTool(def: WebMCPToolDef, options?: RegisterToolOptions): Promise<void>
  getTools(options?: GetToolsOptions): Promise<RegisteredTool[]>
  executeTool(
    tool: RegisteredTool | { name: string },
    args?: Record<string, unknown>,
    options?: ToolExecuteOptions
  ): Promise<ToolResultEnvelope>
}
```

Registration is a loop over the IR, filtered by the current path:

```ts
const refresh = () => {
  unregisterAll()
  const ir = api.ir
  if (!ir || !ctx) return
  const path = currentPath()
  for (const tool of ir.tools) {
    // View-scoped registration: a tool is offered only on its view. This is
    // how the tool set changes per page — each page load (or a host's per-page
    // injection) boots fresh and registers just that view's tools.
    if (tool.ensureView && !routeMatches(tool.ensureView.route, path)) continue
    const controller = new AbortController()
    registrations.push(controller)
    registered.push({ name: tool.name, description: tool.description })
    // registerTool is fire-and-forget, but a rejected native call must NOT
    // become an "Uncaught (in promise) {}" — surface the real reason.
    Promise.resolve(
      ctx.registerTool(
        {
          name: tool.name,
          description: tool.description ?? '',
          inputSchema: tool.inputSchema,
          execute: async (args, options) =>
            toEnvelope(await runTool(tool, args, { signal: options?.signal, currentPath: path })),
        },
        { signal: controller.signal }
      )
    ).catch((e) =>
      console.warn(`[sightkick] registerTool "${tool.name}" rejected: ${describeError(e)}`)
    )
  }
}
```

`ensure_view` does double duty. At compile time it scopes component-name resolution to one view, which is how two views can both have a `ContinueButton`. At runtime it scopes registration, so a tool only exists on pages whose route matches. Every registration gets its own `AbortController`, so a view change tears down exactly the tools that no longer apply.

The effect is a tool list that is a property of where you are. On the menu:

```js
await document.modelContext.getTools()
```

```json
[
  {
    "name": "go_to_cart",
    "description": "Go to the cart from any view via the nav bar's cart button."
  },
  {
    "name": "go_to_menu",
    "description": "Return to the menu from any view via the nav bar logo button."
  },
  {
    "name": "load_scenario",
    "description": "Reload the app on a deterministic error-state fixture."
  },
  { "name": "open_item", "description": "Open a menu item's detail page by name." },
  {
    "name": "read_error_message",
    "description": "Read the first error banner's message text. Returns `value` (string): The error banner's message."
  },
  {
    "name": "read_errors",
    "description": "List every error state currently rendered, by its errorId. Returns `items`: a list of objects with keys {error, message}. One row per rendered error state."
  },
  {
    "name": "read_menu",
    "description": "List the menu items with their prices. Returns `items`: a list of objects with keys {item, price}. The menu rows (item name + price)."
  }
]
```

Seven tools out of thirty. Five of them are globals with no `ensure_view`, so they register everywhere; only `open_item` and `read_menu` are the Menu's own. Note also that the descriptions are longer than what the YAML declares: the compiler appends a result-shape hint from `returns`, so an agent reading the list already knows `read_menu` yields rows of `{item, price}` without calling it.

A few tool calls later, on Checkout:

```json
[
  "apply_promo",
  "go_back_a_step",
  "go_back_to_cart",
  "go_to_cart",
  "go_to_menu",
  "load_scenario",
  "place_order",
  "read_checkout_step",
  "read_error_message",
  "read_errors",
  "read_order_total",
  "submit_delivery_address",
  "submit_payment_details"
]
```

And after the order lands, on Confirmation:

```json
[
  "go_to_cart",
  "go_to_menu",
  "load_scenario",
  "order_again",
  "read_error_message",
  "read_errors",
  "read_order_id"
]
```

`read_menu` and `open_item` are simply gone; `read_order_id` and `order_again` have appeared. This is the same progressive-disclosure argument the sightmap post makes about memory, applied to capability. The agent isn't told about thirty tools and asked to work out which fifteen are legal right now.

## What comes back

Every call returns the same envelope: `ok`, the read (`value` or `items`), any guidance, and a `message` when there's something to say. Three real results from the same tool tell the story.

The happy path:

```json
{
  "guidance": [
    {
      "tool": "read_order_total",
      "reason": "read the discounted total while it still exists — it doesn't survive to the confirmation",
      "when": "now"
    }
  ],
  "ok": true,
  "value": "Total: $18.92"
}
```

Calling it a second time, with the promo already applied:

```json
{
  "guidance": [ ... ],
  "message": "guard satisfied; steps skipped (already applied)",
  "ok": true,
  "skipped": true,
  "value": "Total: $18.92"
}
```

`ok: true` with `skipped: true` is the correct answer to "apply this promo" when the promo is already applied. The read still runs, so the caller gets the current total either way, and an agent retrying after a timeout doesn't double-apply anything.

And a code the app rejects:

```json
{
  "message": "waitFor: timed out after 5000ms for query [\".checkout .promo-applied\"]",
  "ok": false
}
```

That third result took a bug fix to get. The tool originally waited on the totals block, which is on screen whether the promo lands or not, so an invalid code returned `ok: true` with an unchanged total and no complaint. The fix wasn't in the tool at all: we added a `PromoAppliedLabel` component to the corpus, mapped to the confirmation line the app only renders on success, and pointed `wait_for` at that instead.

Worth stating plainly, because it generalizes: **a tool is exactly as honest as the component it waits on.** `wait_for` targeting something that exists in both the success and failure states is the single easiest way to write a tool that always passes.

## Watching it run

`apply_promo`, before and after, plus the totals block it reads from:

<div data-widget="sightkick-frames" data-figure="tool">
<img src="/blog/images/sightkick/tool-04-promo-before.png" alt="The Checkout Review step before the promo call, with an empty promo code field and a total of $23.65." />
</div>

And the full `purchase` journey, fourteen tool calls from the menu to a confirmed order id, one frame per stage with the guidance breadcrumb that pointed there:

<div data-widget="sightkick-frames" data-figure="journey">
<img src="/blog/images/sightkick/tool-01-menu.png" alt="The Burrito Co. menu, five items with prices, at the start of the purchase journey." />
</div>

The breadcrumb after `add_item_to_cart` is the one to look at:

```
commit the item — this lands you on the cart, not back on the menu
```

That sentence exists because the app does something an agent would otherwise have to discover: adding an item navigates. Without the breadcrumb, the agent's next move is a snapshot to find out where it ended up, then a decision about whether it needs a `go_to_cart` call it doesn't. With it, `add_item_to_cart` returns `ok: true`, `read_cart` is the suggested next call, and the round trip never happens. Multiply by fourteen steps.

## Testing: features, plans, and no agent in the loop

Once an app's actions are named, typed, and callable, they are also a test harness, and this is where sightkick's day job actually is.

The pipeline has four layers. A **scenario** is a `.feature` file: intent in business language, unaware sightkick exists. The **corpus** is what the app is. The **tool layer** is what you can do and the order people do it in. A **plan** is a scenario resolved against that tool layer, each Gherkin line mapped to a tool call and an expectation, hashed and checked in.

Burrito Co.'s `features/purchase.feature`:

```gherkin
Feature: Order a burrito
  As a hungry customer
  I want to customize an item and pay for it
  So that I get a confirmed order

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

An agent reads that once and resolves it into `plans/purchase.plan.json`. Three of the thirteen steps:

```json
{
  "gherkin": "And I customize \"protein\" as \"steak\"",
  "tool": "customize_item",
  "params": { "group": "protein", "option": "steak" },
  "expect": { "value": { "equals": "steak" } }
},
{
  "gherkin": "Then the cart holds one line for \"Classic Burrito\" at \"$21.90\"",
  "tool": "read_cart",
  "expect": {
    "list": { "length": 1, "contains": { "item": "Classic Burrito", "line_total": "$21.90" } }
  }
},
{
  "gherkin": "And I apply the promo code \"BURRITO20\"",
  "tool": "apply_promo",
  "params": { "code": "BURRITO20" },
  "expect": { "value": { "contains": "$18.92" } }
}
```

No selectors, and no step-definition codebase to rot alongside the spec. Every run after that first resolution is `node scripts/run-plan.mjs examples/burrito/plans/purchase.plan.json`, run from the sightkick repo root: no model call, no tokens.

Two hashes keep that honest. `scenario.hash` covers the `.feature` text; `irHash` covers the compiled manifest. Both are recomputed on every run, and a mismatch stops the run rather than reporting a misleading failure. Editing one tool's description is enough to trip it:

```
$ sed -i '' 's/List the menu items with their prices\./List the current menu items and their prices./' examples/burrito/.sightkick/menu.yaml
$ sightkick build examples/burrito -o /tmp/burrito-drift.ir.json
✓ wrote 30 tool(s) to /tmp/burrito-drift.ir.json

$ node scripts/run-plan.mjs examples/burrito/plans/purchase.plan.json
✗ examples/burrito's compiled manifest has changed since this plan was stamped — re-plan (or pass --stale-ok).
```

The hash is over the compiled IR rather than the raw YAML, so a comment-only edit to the same file changes nothing.

Building these three plans surfaced a real bug. The Review step showed `Total: $18.92` with the promo line visible, and `read_order_total` confirmed it. `place_order` succeeded. The confirmation screen then reported **Total Charged $23.65**, the undiscounted price. The discount never reaches the confirmation. Both screenshots are in the widgets above, and the journey transcript has the two reads back to back. This is exactly the class of thing a human clicking through says "yep, ordered" to, because the confirmation screen is the one nobody reads.

Read the source, file issues, or point an agent at your own app: [github.com/sightmap/sightkick](https://github.com/sightmap/sightkick).
