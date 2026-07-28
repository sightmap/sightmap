---
title: 'Sightmaps: the runtime map of your app'
excerpt: 'A sightmap gives every view, component, and API request in your app a semantic name, so your agent sees meaning instead of DOM noise and carries forward the instructions for how to operate your app.'
topic: 'research'
date: '2026-07-28'
author: 'Clint Ayres'
slug: 'sightmaps'
draft: true
image: '/blog/og/sightmaps.png'
---

Today we're open-sourcing [Sightmap](https://github.com/sightmap/sightmap) (yes, *sight*map, not sitemap), a runtime map that teaches the agents you run against your app how to operate it: every view, component, and API request gets a semantic name and a memory of how it behaves.

To see why you'd want one, point a coding or verification agent at your running app: each turn it reads the accessibility tree like it's seeing it for the first time, guessing which button routes to the next view, whether an error is real or cosmetic, how a fussy date picker takes input. The agent that solved this exact view in a past run starts over, and that re-deriving is slow and burns tokens in the inner loop where it verifies its own fix or reproduces a bug.

A sightmap gives that learned understanding somewhere to live, durable and shared, so the next agent reads a name and a note instead. If you already point agent-driven browsers at your app during development, that makes them **faster, cheaper, and more reliable**.

Agents maintain the sightmap themselves as they loop through the app. It's scoped for the agents you run against your own codebase, not a UI you stand up for your customers' agents. That's the problem [WebMCP](https://webmachinelearning.github.io/webmcp/) solves: a page declaring its own callable actions for third-party agents to invoke.

Let's look at an example of an accessibility tree for a checkout form. Toggle the widget to **with a sightmap** to see an agent-facing tree appear, CheckoutForm, CardFieldset, ContinueButton, plus a memory note explaining why Continue is disabled. It's about the same size as the raw tree; what changes is that every line carries a name and a memory the agent would otherwise re-derive.

<div data-widget="sightmap-snapshot" data-figure="checkout">
<pre><code>[generic]
  [generic]
    [generic] "Checkout"
    [generic]
      [generic] "Card number"
      [textbox] "4242 4242 4242 4242"
      [generic] "Expiry"
      [textbox] "12 / 28"
      [generic] "CVC"
      [textbox] "123"
    [generic]
      [generic] "Promo code"
      [textbox]
      [button] "Apply"
    [generic] "Total: $42.00"
    [button] "Continue" [disabled]</code></pre>
</div>

The accessibility tree is a specialized representation of the app built for assistive technologies. Sightmap builds the equivalent for agents: a runtime component tree that takes the accessibility tree as one input and annotates it with the names and memory from your `.sightmap/` corpus. That agent-facing tree is what's being developed at [sightmap.org](https://sightmap.org/) through the open-source [sightmap](https://github.com/sightmap/sightmap) project.

## A sightmap is your app's agent-facing layer

A sightmap is a directory of small YAML files, checked into your repo alongside the codebase. `sitemap.xml` tells a search crawler how to _crawl_ your public pages; a `.sightmap/` teaches the agents working on your app how to _use_ it.

Sightmaps are learned from the running app: an agent drives it, and every interactive element, error state, and API request gets a name and an optional memory field. When every interactive element is named and every true-positive error state defined, the map is complete.

### Component

```yaml
version: 1
components:
  - name: CheckoutForm
    selector:
      - '[data-component="CheckoutForm"]'
      - '.checkout-form'
    source: src/components/checkout/CheckoutForm.tsx
    memory:
      - 'CardFieldset validates on blur. Tab or click off a field before submitting, or ContinueButton stays disabled.'
```

### Request

```yaml
version: 1
requests:
  - name: SubmitCheckout
    route: /api/checkout
    method: POST
    description: Charges the card and creates the order
    source: src/api/checkout.ts
    request:
      fields:
        - name: cardToken
          type: string
        - name: total
          type: number
    response:
      fields:
        - name: orderId
          type: string
        - name: status
          type: string
```

Those two examples cover the fields you'll reach for most; the full schema, every field, type, and matching rule, lives in the [spec](https://github.com/sightmap/sightmap/blob/main/spec/v1/sightmap.schema.json).

## Sightmap browser: our reference implementation

`sightmap browser` isn't a new automation framework competing with Playwright, Selenium, or Chrome DevTools MCP. It speaks the same Chrome DevTools Protocol (CDP) underneath them, and drives its own Chrome-for-Testing instance with `sightmap browser start`. It ships alongside the CLI in the open-source [`@sightmap/sightmap`](https://github.com/sightmap/sightmap) package:

```bash
npm install -g @sightmap/sightmap
```

It includes a companion browser extension: a live, interactive overlay showing page coverage and the event stream in real time.

<figure class="shot shot-wide">
<img src="/blog/images/sightmaps/overlay-checkout.png" alt="The sightmap browser extension on the Burrito Co. checkout: a component-path tooltip over the page, and a side panel listing the hovering path and an event log." />
<figcaption>The `sightmap browser` extension: a live, interactive view of the coverage and event stream the sightmap tools see.</figcaption>
</figure>

The Sightmap CLI ships with authoring tools that visualize this coverage and help you converge on a complete map: it scores each page and flags unmapped gaps or stale references until every interactive element is named.

For browser operation what changes is how an agent addresses the page. Instead of scanning the DOM for a selector like `button[data-testid="checkout-continue"]` on every page, it drives the page by component name from start to screenshot:

```bash
sightmap browser start --url http://localhost:5173/checkout   # launch Chrome + session
sightmap snapshot                                             # annotated, named tree + coverage
sightmap browser fill 'CardNumberInput' '4242 4242 4242 4242' # target components by name
sightmap browser click 'ContinueButton'                       # not a CSS selector
sightmap browser screenshot --component CheckoutForm --out checkout.png
```

Each name resolves against the live component tree on the call, so the same command keeps working as the markup shifts underneath it.

## Skills load context up front; sightmaps disclose it on demand

Skills are a simple, elegant solution for agent memory, but they dump all their context into the prompt up front, regardless of what the agent is looking at, so stale or irrelevant rules load anyway and the agent has to reason about when they apply. A sightmap turns that into progressive disclosure: the interface surfaces only the active components and their memories, scoped to what the agent is doing right now.

How it's delivered is the point: memory surfaces only for the components matched on the current page, view-level notes under the `[View: …]` header, component notes traveling with the component in the annotated tree. A component the agent isn't looking at contributes nothing to context. The same mechanism runs on the review side: [Subtext](/), our agent session review tool, injects matching entries into snapshot and network-trace output as it reviews a captured session. This is where the token math favors a sightmap. Loaded as a skill, the whole burrito corpus is about 2,300 tokens the agent carries on every turn, on every view; progressive disclosure surfaces only the active view, about 500. The gap widens as the app grows: a skill's cost climbs with every screen you add, a sightmap's stays bounded by the one you're on.

<div data-widget="skill-vs-sightmap"></div>

## Memory records the runtime behavior source code hides

As we developed the sightmap spec, we realized semantic names didn't explain a component's runtime behavior. Agents need to know _how_ to operate the app at the component level, and they started annotating our sightmaps with notes about it themselves. We formalized that into memory.

Take a DeliveryDatePicker on a checkout flow. It looks like a normal date-picker dialog: click the field, a calendar opens, click a day. What's not obvious is you can operate it by typing into a text field in the format `MM/DD/YYYY`. When an agent discovers that shortcut, memory preserves it.

The next agent to reach this screen skips the calendar entirely:

```bash
sightmap browser fill 'DateTrigger' '12/28/2026'
```

One command, no clicking through a month grid to find a day cell, because the memory that made this possible is attached to the component, not buried in a chat transcript from whichever agent discovered it.

This isn't a hypothetical shape. Memory attaches at four levels: file, view, component, and request. Each entry is one plain sentence, surfaced whenever that scope is active. The spec's own date-picker example carries exactly the note our story needed:

```yaml
- name: DepartureDatePicker
  selector: '[data-picker="departure"]'
  memory:
    - Accepts typed YYYY-MM-DD — skips the calendar
    - Past dates render but are aria-disabled
```

A request memory reads the same way, `Rate-limited to 10 requests/min per user; returns 429 beyond that`, except it surfaces in the network-trace view instead of the DOM.

## What's next: static analysis and near-free navigation

The bigger bet, which we haven't built yet, is static analysis: pathing through an app without spending agent tokens on the intermediate navigation steps. If the runtime map already knows that clicking Checkout leads to `/checkout`, that the form has fields with prerequisites, and that clicking Continue leads to `/confirmation`, an agent can build a script that walks that path for free. That moves the agent to where you actually want it thinking, and spends tokens only on the pages that matter: the ones with new changes, the ones with decisions, the unmapped sections. A big part of this is low-tier models navigating complex apps with a fraction of the friction they'd otherwise hit.

It also lines up with what we're building at Subtext on the agent session-review side, where knowing which errors are real versus cosmetic (a true-positive) is what separates signal from noise. And on the find-and-fix side, a sightmap turns "jump from this live view into the code that renders it" into a lookup, and gives an agent a path from entry point to component to test when it needs to change a UI file and verify the change against the running app.

There's a standards question sitting underneath this too. Browser vendors are already moving toward agent-facing HTML: WebMCP, the effort mentioned above, is an early W3C proposal from Google and Microsoft. Different problem, same direction: less agent guesswork, more of the app just telling you what it is.

Read the source, file issues, or point an agent at your own app: [github.com/sightmap/sightmap](https://github.com/sightmap/sightmap).
