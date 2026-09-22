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
  // The real screen this call produced. Several steps legitimately share a
  // capture: a read doesn't change the screen the write before it produced.
  shot: string
  shotAlt: string
  params?: Record<string, string>
  guard?: string
  queries: string[]
  reads?: { component: string; selector: string; property: string }[]
  result: string
  expect: string
  guidance?: string
}

const IMG = '/blog/images/sightkick'

const STEPS: Step[] = [
  {
    gherkin: 'Given the menu lists five items',
    tool: 'read_menu',
    ensureView: 'Menu',
    shot: `${IMG}/tool-01-menu.png`,
    shotAlt: 'The Burrito Co. menu, five items with prices.',
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
    shot: `${IMG}/tool-02-item-before.png`,
    shotAlt: 'Classic Burrito detail page, chicken selected, quantity 1.',
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
    shot: `${IMG}/tool-03-item-after.png`,
    shotAlt: 'The same detail page, steak now the selected protein.',
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
    shot: `${IMG}/journey-02-item.png`,
    shotAlt: 'Classic Burrito detail page, steak selected, quantity 2, the button reading Add 2 to Cart $21.90.',
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
    shot: `${IMG}/journey-03-cart.png`,
    shotAlt: 'The cart holding one line, 2x Classic Burrito at $21.90.',
    queries: ['AddToCartButton'],
    result: 'ok: true, wait_for view: Cart',
    expect: 'ok: true',
    guidance: 'commit the item — this lands you on the cart, not back on the menu',
  },
  {
    gherkin: 'Then the cart holds one line for "Classic Burrito" at "$21.90"',
    tool: 'read_cart',
    ensureView: 'Cart',
    shot: `${IMG}/journey-03-cart.png`,
    shotAlt: 'The same cart line, read back: 2x Classic Burrito at $21.90.',
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
    shot: `${IMG}/journey-04-checkout-delivery.png`,
    shotAlt: 'Checkout step 1 of 3, Delivery, with the address fields.',
    queries: ['CheckoutButton'],
    result: 'ok: true',
    expect: 'ok: true',
  },
  {
    gherkin: 'And I enter the delivery address "123 Main St", "Denver", "CO", "80203"',
    tool: 'submit_delivery_address',
    ensureView: 'Checkout',
    shot: `${IMG}/journey-05-checkout-payment.png`,
    shotAlt: 'Checkout step 2 of 3, Payment, with the card fields.',
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
    shot: `${IMG}/journey-06-checkout-review.png`,
    shotAlt: 'Checkout step 3 of 3, Review, before the promo code.',
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
    shot: `${IMG}/tool-05-promo-after.png`,
    shotAlt: 'The Review step with BURRITO20 applied and the total at $18.92.',
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
    shot: `${IMG}/tool-06-promo-detail.png`,
    shotAlt: 'Close-up of the order totals: subtotal, promo discount, tax, and a total of $18.92.',
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
    shot: `${IMG}/journey-07-confirmation.png`,
    shotAlt: 'The order confirmation screen, with an order id and Total Charged $23.65.',
    queries: ['PlaceOrderButton'],
    result: 'ok: true',
    expect: 'ok: true',
  },
  {
    gherkin: 'Then I get an order id',
    tool: 'read_order_id',
    ensureView: 'Confirmation',
    shot: `${IMG}/journey-07-confirmation.png`,
    shotAlt: 'The same confirmation screen, the order id read back off it.',
    queries: [],
    reads: [{ component: 'OrderIdDisplay', selector: '.order-id', property: 'orderId ← text' }],
    result: '"ORD-1757329143100"',
    expect: 'value.contains: "ORD-"',
  },
]

const ADVANCE_MS = 1000

export default function FeatureTrace() {
  const [i, setI] = useState(0)
  const [playing, setPlaying] = useState(false)
  const figureRef = useRef<HTMLElement>(null)
  const activeLineRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLOListElement>(null)
  const hasStarted = useRef(false)

  // The feature list is capped and scrollable (see .ftrace__feature-list in
  // index.css), so the active line has to be kept in view by hand as the
  // trace advances.
  //
  // This nudges the list's own scrollTop rather than calling scrollIntoView,
  // which walks up and scrolls every scrollable ancestor including the
  // document: on mount, with the widget below the fold, that yanked the whole
  // page down to the widget on every load.
  useEffect(() => {
    const line = activeLineRef.current
    const list = listRef.current
    if (!line || !list) return
    const lineBox = line.getBoundingClientRect()
    const listBox = list.getBoundingClientRect()
    if (lineBox.top < listBox.top) {
      list.scrollTop -= listBox.top - lineBox.top
    } else if (lineBox.bottom > listBox.bottom) {
      list.scrollTop += lineBox.bottom - listBox.bottom
    }
  }, [i])

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

  // `i` has to be a dependency: without it the effect never re-runs as the
  // step changes (isAdvancing stays true the whole way), so exactly one
  // timer is ever scheduled and playback stops after a single step.
  useEffect(() => {
    if (!isAdvancing) return
    const t = setTimeout(() => setI((n) => Math.min(last, n + 1)), ADVANCE_MS)
    return () => clearTimeout(t)
  }, [isAdvancing, i, last])

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
        <ol className="ftrace__feature-list" ref={listRef}>
          {STEPS.map((s, n) => (
            <li key={n}>
              <button
                type="button"
                ref={n === i ? activeLineRef : undefined}
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

      <div className="ftrace__stage">
        {/* Every frame is mounted and only the active one is shown, rather
            than swapping one img's src. Swapping meant each frame was
            fetched and decoded the first time it came up, which at a 1s
            cadence lands as a visible blank-then-snap. Mounted up front,
            the browser has them all decoded before playback reaches them,
            and the nine unique files are deduped by the HTTP cache. */}
        <div className="ftrace__shot">
          {STEPS.map((s, n) => (
            <img
              key={n}
              src={s.shot}
              alt={n === i ? s.shotAlt : ''}
              aria-hidden={n !== i}
              className={n === i ? 'is-current' : undefined}
            />
          ))}
        </div>

        <div className="ftrace__panes">
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
        </div>
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
