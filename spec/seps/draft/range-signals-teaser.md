# Signals as a ladder: points → roll-ups → areas (teaser)

> Pre-SEP sketch on the `proposal/range-signals` branch. Not a spec proposal yet —
> the mental model we want to land before writing it up. Companion to the
> "App Areas in the Sightmap" note. Expect to iterate.

## Thesis

One primitive — the **signal** — at every level. We start from the point signals
the pipeline already emits and *roll them up* into progressively higher-level
**time-range signals**, all the way to "areas." Areas aren't a special construct;
they're just the broadest time-range signals, written with the same rule shape as
everything else.

## The ladder

- **L0 · primitives (points)** — `navigate`, `click`, `network`, `exception`, `console`.
- **L1 · behavior** — `rage-click`, `dead-click`, `slow/failed request`.
- **L2 · intent** — `rage-declined`, `checkout-abandoned`, `recovery`.
- **L3 · cross-navigation** — `ping-pong`, `backtrack`, `redirect-bounce`.
- **L4 · areas** — `browse`, `checkout`, `auth` (broad membership spans).

Each rung is composed from the rung below.

## One session, read top-to-bottom

Time flows down. Each lane is a live time-range signal; the right column is the
ordinary event transcript, so the bars are pure annotation.

```text
 t │ B C F P R │ transcript
───┼───────────┼───────────────────────────────
 0 │ ┃         │ nav Home
 4 │ ┃         │ click Product → nav Product
 9 │ ┃         │ click AddToCart · net 200
12 │ ┃   ┏ ┏   │ nav Cart          ┐
14 │ ┃   ┃ ┃   │ nav Shipping      │ Cart⇄Shipping
16 │ ┃   ┃ ┃   │ nav Cart          │
18 │ ┃   ┃ ┗   │ nav Shipping      ┘ → ping-pong
20 │ ┗ ┏ ┃     │ nav Checkout        ← area handoff
24 │   ┃ ┃     │ change Card
28 │   ┃ ┃   ┏ │ click Pay · net DECLINED  ┐
29 │   ┃ ┃   ┃ │ click Pay · net DECLINED  │ rage-click
30 │   ┃ ┃   ┃ │ click Pay · net DECLINED  ┘
31 │   ┃ ┗   ┗ │ net DECLINED → rage-declined
34 │   ┃       │ ✗ exception "gateway error"
38 │   ┃       │ click Pay · net 200 OK → recovery
40 │   ┗       │ nav Confirmation

Lanes:  B browse · C checkout · F frustration · P ping-pong · R rage-declined
```

## What to notice

- **Roll-up.** Each higher signal is composed from lower ones: `click`s → `rage-click` → `rage-declined`.
- **Nesting.** `click` ⊂ `rage-click` ⊂ `rage-declined` ⊂ `area:checkout`.
- **Arbitrary overlap.** `frustration` (F) spans the browse→checkout handoff at t20 — it belongs fully to *neither* area. Time-range signals overlap freely; they don't only nest.
- **Cross-navigation.** `ping-pong` (P) is a signal no single event can express — it exists only over the *sequence* of navigations.

## Definitions (footnotes)

Every signal below uses the **same shape** — a `match:` of bindings over the input
stream, an optional `where:` gate, and an `emit:` projection. Behavior, intent,
cross-navigation, and areas are all just signals; there is no special "app area"
construct. Higher signals reference lower signals by name exactly as they
reference primitives, which is what makes the ladder compose.

Syntax is illustrative. A couple of pieces are deliberately hand-wavy here and
flagged inline — they're what a real SEP would pin down.

**point signal** — not a rule: the primitive events the pipeline emits directly
(`navigate`, `click`, `network`, `exception`, `console`). Each carries built-in
attributes fixed by its kind (a `click` has a `selector`/target; a `network` has
`method`/`status`/…), plus any properties contributed by the sightmap entity it
matched.

**rage-click** (L1) — ≥3 rapid clicks on one target.

```yaml
- name: rage-click
  by: click.target            # one instance per clicked target
  within: 2s
  match: { taps: click+ }
  where: taps.count >= 3
  emit:
    tags: [rage-click]
    attrs: { target: click.target, taps: taps.count }   # multi-input → mints a span
```

**ping-pong** (L3) — the user bounces between two views. A cross-navigation
signal: it exists only over the `navigate` subsequence.

```yaml
- name: ping-pong
  within: 30s
  match: { hops: navigate+ }
  where: hops.distinct == 2 and hops.count >= 4   # (hand-wavy: "exactly two views" needs a partition rule)
  emit: { tags: [ping-pong], attrs: { views: hops.distinct } }
```

**rage-declined** (L2) — rage on Pay, then a declined payment. Note it composes
the **`rage-click` signal** above, not raw clicks — same reference mechanism as a
primitive.

```yaml
- name: rage-declined
  within: 10s
  match:
    rage:    rage-click where target has PayButton    # ← references the L1 signal
    decline: network    where name == CheckoutPay and outcome == declined
  where: exists rage and exists decline
  emit: { tags: [defect], attrs: { taps: rage.taps, amount: decline.amount } }
```

**frustration** (meta) — rolls up cross-nav and intent evidence into one span.
Because it spans from the ping-pong through the rage, it crosses the
browse→checkout handoff — the example of a signal that overlaps areas without
nesting.

```yaml
- name: frustration
  match: { evidence: (ping-pong | rage-declined)+ }
  where: evidence.count >= 2
  emit:
    tags: [frustration]
    attrs: { span: [evidence.first.when, evidence.last.when] }
```

**recovery** (L2) — a struggle that ends in success; the positive counterpart, so
it isn't all defects.

```yaml
- name: recovery
  within: 20s
  match:
    struggle: rage-declined
    ok:       network where name == CheckoutPay and outcome == ok
  where: exists struggle and exists ok
  emit: { tags: [recovery] }
```

**area:checkout** (L4) — the punchline: an area is *the same rule shape*. A run of
member navigations, closed by navigating away; the span is the area's extent.

```yaml
- name: area.checkout
  match:
    body: navigate+ where name in [Checkout, Shipping, Payment, Confirmation]
    away: navigate  where not name in [Checkout, Shipping, Payment, Confirmation]
  where: exists body and exists away
  emit:
    tags: [area:checkout]
    attrs: { span: [body.first.when, body.last.when] }
```

`area:browse` is identical in shape, with its own member set
(`[Home, Product, Cart, Shipping]`). No new construct — just another signal.
