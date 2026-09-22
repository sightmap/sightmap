import { useEffect, useRef, useState } from 'react'

// Playback widget for the "building-in-layers" post: a real 13-step plan
// (examples/burrito/plans/purchase.plan.json in the sightkick repo) stepped
// one Gherkin line at a time, with the tool call and the corpus resolution
// that line compiled to shown alongside it. Every value below is transcribed
// from that plan plus the .sightkick/ and .sightmap/ YAML it was compiled
// from. The point is that all three layers move together: one Gherkin line,
// one tool call, one set of named components.

type Step = {
  gherkin: string
  tool: string
  ensureView: string
  params?: Record<string, string>
  guard?: string
  queries: string[]
  reads?: { component: string; selector: string; property: string }[]
  result: string
  expect: string
  guidance?: string
}

const STEPS: Step[] = [
  {
    gherkin: 'Given the menu lists five items',
    tool: 'read_menu',
    ensureView: 'Menu',
    queries: [],
    reads: [
      { component: 'MenuCard', selector: '.menu-card', property: 'itemName ← MenuCardName.text' },
      { component: 'MenuCard', selector: '.menu-card', property: 'price ← MenuCardPrice.text' },
    ],
    result:
      '5 rows: Classic Burrito $10.95 · Chicken Burrito Bowl $11.50 · Veggie Tacos $9.75 · Chips & Guacamole $4.95 · Steak Quesadilla $13.25',
    expect: 'list.length: 5',
  },
  {
    gherkin: 'When I open "Classic Burrito"',
    tool: 'open_item',
    ensureView: 'Menu',
    params: { name: 'Classic Burrito' },
    queries: ['MenuCard[itemName*="Classic Burrito" i]'],
    result: 'ok: true, wait_for view: ItemDetail',
    expect: 'ok: true',
    guidance: "customizations only exist on an item's own detail page",
  },
  {
    gherkin: 'And I customize "protein" as "steak"',
    tool: 'customize_item',
    ensureView: 'ItemDetail',
    params: { group: 'protein', option: 'steak' },
    queries: ['CustomizationGroup[groupName*="protein" i] OptionButton[label*="steak" i]'],
    reads: [{ component: 'OptionButton', selector: '.option-btn', property: 'label ← text' }],
    result: '"steak"',
    expect: 'value.equals: "steak"',
  },
  {
    gherkin: 'And I increase the quantity to 2',
    tool: 'increase_item_quantity',
    ensureView: 'ItemDetail',
    queries: ['QtyButton[label="+"]'],
    reads: [
      {
        component: 'AddToCartButton',
        selector: 'button.add-to-cart-btn',
        property: 'label ← text',
      },
    ],
    result: '"Add 2 to Cart · $21.90"',
    expect: 'value.contains: "Add 2 to Cart"',
  },
  {
    gherkin: 'And I add it to the cart',
    tool: 'add_item_to_cart',
    ensureView: 'ItemDetail',
    queries: ['AddToCartButton'],
    result: 'ok: true, wait_for view: Cart',
    expect: 'ok: true',
    guidance: 'commit the item — this lands you on the cart, not back on the menu',
  },
  {
    gherkin: 'Then the cart holds one line for "Classic Burrito" at "$21.90"',
    tool: 'read_cart',
    ensureView: 'Cart',
    queries: [],
    reads: [
      { component: 'CartItem', selector: '.cart-item', property: 'itemName ← CartItemName.text' },
      { component: 'CartItem', selector: '.cart-item', property: 'price ← CartItemPrice.text' },
    ],
    result: '[{ item: "Classic Burrito", line_total: "$21.90" }]',
    expect: 'list.length: 1, list.contains',
  },
  {
    gherkin: 'When I check out',
    tool: 'go_to_checkout',
    ensureView: 'Cart',
    queries: ['CheckoutButton'],
    result: 'ok: true',
    expect: 'ok: true',
  },
  {
    gherkin: 'And I enter the delivery address "123 Main St", "Denver", "CO", "80203"',
    tool: 'submit_delivery_address',
    ensureView: 'Checkout',
    params: { street: '123 Main St', city: 'Denver', state: 'CO', zip: '80203' },
    guard: 'wait CheckoutSteps[activeStep*="Delivery" i]',
    queries: ['StreetField', 'CityField', 'StateField', 'ZipField', 'ContinueButton'],
    reads: [
      { component: 'CheckoutSteps', selector: '.checkout-steps', property: 'activeStep ← text' },
    ],
    result: '"Payment"',
    expect: 'value.contains: "Payment"',
  },
  {
    gherkin: 'And I pay with card "4242 4242 4242 4242" expiring "09/26"',
    tool: 'submit_payment_details',
    ensureView: 'Checkout',
    params: { card_number: '4242 4242 4242 4242', expiry: '09/26' },
    guard: 'wait CheckoutSteps[activeStep*="Payment" i]',
    queries: [
      'CardNumberField',
      'CardExpiryField',
      'CardCvvField',
      'CardNameField',
      'ContinueButton',
    ],
    reads: [
      { component: 'CheckoutSteps', selector: '.checkout-steps', property: 'activeStep ← text' },
    ],
    result: '"Review"',
    expect: 'value.contains: "Review"',
  },
  {
    gherkin: 'And I apply the promo code "BURRITO20"',
    tool: 'apply_promo',
    ensureView: 'Checkout',
    params: { code: 'BURRITO20' },
    guard: 'absent: PromoField',
    queries: ['PromoField', 'ApplyPromoButton', 'PromoAppliedLabel'],
    reads: [
      {
        component: 'ReviewTotals',
        selector: '.review-totals',
        property: 'total ← ReviewTotalsAmount.text',
      },
    ],
    result: '"Total: $18.92"',
    expect: 'value.contains: "$18.92"',
    guidance:
      "read the discounted total while it still exists — it doesn't survive to the confirmation",
  },
  {
    gherkin: 'Then the order total is "$18.92"',
    tool: 'read_order_total',
    ensureView: 'Checkout',
    queries: [],
    reads: [
      {
        component: 'ReviewTotals',
        selector: '.review-totals',
        property: 'total ← ReviewTotalsAmount.text',
      },
    ],
    result: '"Total: $18.92"',
    expect: 'value.contains: "$18.92"',
    guidance: 'submit from Review; a card ending 0000 is declined here, not earlier',
  },
  {
    gherkin: 'When I place the order',
    tool: 'place_order',
    ensureView: 'Checkout',
    queries: ['PlaceOrderButton'],
    result: 'ok: true',
    expect: 'ok: true',
  },
  {
    gherkin: 'Then I get an order id',
    tool: 'read_order_id',
    ensureView: 'Confirmation',
    queries: [],
    reads: [{ component: 'OrderIdDisplay', selector: '.order-id', property: 'orderId ← text' }],
    result: '"ORD-1757329143100"',
    expect: 'value.contains: "ORD-"',
  },
]

const ADVANCE_MS = 1600

export default function FeatureTrace() {
  const [i, setI] = useState(0)
  const [playing, setPlaying] = useState(false)
  const figureRef = useRef<HTMLElement>(null)
  const hasStarted = useRef(false)

  // Start playback the first time the widget scrolls into view, so it
  // doesn't finish its run before a reader gets to it, and never at all for
  // a reader who has asked for less motion.
  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const el = figureRef.current
    if (!el) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !hasStarted.current) {
          hasStarted.current = true
          setPlaying(true)
        }
      },
      { threshold: 0.4 }
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const last = STEPS.length - 1
  // `playing` is the user's intent; whether that intent is currently
  // advancing anything is derived (there's nothing left to schedule once
  // the trace reaches its last step), so reaching the end never needs a
  // setState call from inside the effect itself.
  const isAdvancing = playing && i < last

  useEffect(() => {
    if (!isAdvancing) return
    const t = setTimeout(() => setI((n) => Math.min(last, n + 1)), ADVANCE_MS)
    return () => clearTimeout(t)
  }, [isAdvancing, last])

  const step = STEPS[i]

  function goTo(n: number) {
    setPlaying(false)
    setI(Math.max(0, Math.min(last, n)))
  }

  function togglePlay() {
    if (i >= last) {
      setI(0)
      setPlaying(true)
      return
    }
    setPlaying((p) => !p)
  }

  return (
    <figure className="ftrace" ref={figureRef}>
      <div className="ftrace__pane ftrace__pane--feature">
        <div className="ftrace__pane-label">purchase.feature</div>
        <ol className="ftrace__feature-list">
          {STEPS.map((s, n) => (
            <li key={n}>
              <button
                type="button"
                className={
                  'ftrace__feature-line' + (n === i ? ' is-current' : n < i ? ' is-done' : '')
                }
                onClick={() => goTo(n)}
                aria-current={n === i ? 'step' : undefined}
              >
                <span className="ftrace__feature-mark" aria-hidden="true">
                  {n < i ? '✓' : n === i ? '▸' : ''}
                </span>
                {s.gherkin}
              </button>
            </li>
          ))}
        </ol>
      </div>

      <div className="ftrace__pane ftrace__pane--tool" aria-live="polite">
        <div className="ftrace__pane-label">tool call</div>
        <div className="ftrace__tool-name">
          {step.tool}
          <span className="ftrace__tool-view"> ensure_view: {step.ensureView}</span>
        </div>
        {step.params && (
          <div className="ftrace__kv">
            {Object.entries(step.params).map(([k, v]) => (
              <div key={k}>
                <span className="ftrace__kv-key">{k}</span> = {v}
              </div>
            ))}
          </div>
        )}
        {step.guard && <div className="ftrace__guard">guard: {step.guard}</div>}
        <div className="ftrace__result">
          <span className="ftrace__result-label">result</span> {step.result}
        </div>
        <div className="ftrace__expect">
          <span className="ftrace__result-label">expect</span> {step.expect}
        </div>
        {step.guidance && <div className="ftrace__guidance">guidance → {step.guidance}</div>}
      </div>

      <div className="ftrace__pane ftrace__pane--corpus" aria-live="polite">
        <div className="ftrace__pane-label">corpus</div>
        {step.queries.length === 0 && !step.reads?.length && (
          <div className="ftrace__corpus-empty">no component addressed, a pure read</div>
        )}
        {step.queries.map((q, n) => (
          <div className="ftrace__query" key={n}>
            {q}
          </div>
        ))}
        {step.reads?.map((r, n) => (
          <div className="ftrace__read" key={n}>
            <span className="ftrace__read-component">{r.component}</span>
            <span className="ftrace__read-selector">{r.selector}</span>
            <span className="ftrace__read-property">{r.property}</span>
          </div>
        ))}
      </div>

      <div className="ftrace__controls">
        <button
          type="button"
          className="ftrace__nav"
          onClick={() => goTo(i - 1)}
          disabled={i === 0}
          aria-label="Previous step"
        >
          ←
        </button>
        <button
          type="button"
          className="ftrace__nav ftrace__nav--play"
          onClick={togglePlay}
          aria-label={isAdvancing ? 'Pause' : i >= last ? 'Replay' : 'Play'}
        >
          {isAdvancing ? '❙❙' : i >= last ? '↻' : '▶'}
        </button>
        <button
          type="button"
          className="ftrace__nav"
          onClick={() => goTo(i + 1)}
          disabled={i === last}
          aria-label="Next step"
        >
          →
        </button>
        <span className="ftrace__count">
          step {i + 1} of {STEPS.length}
        </span>
      </div>
      <figcaption>
        Every frame is the real plan: `examples/burrito/plans/purchase.plan.json`, resolved against
        `.sightkick/` tools and the `.sightmap/` corpus they name.
      </figcaption>
    </figure>
  )
}
