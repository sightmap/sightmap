import { useState } from 'react'

// Frame-sequence stepper for the sightkick post: a numbered set of real
// screenshots captured mid-session, stepped one at a time. Two figures:
// "tool" walks one tool call's before/after; "journey" walks a full
// purchase across all five views. Each frame pairs the screenshot with the
// real guidance breadcrumb the prior tool call returned, so a "journey" —
// which never executes anything itself — is shown as what it actually is:
// the breadcrumbs an agent follows, not a recording of clicks.

type Frame = {
  src: string
  alt: string
  caption: string
  breadcrumb?: string
}

type FigureData = { frames: Frame[] }

const IMG = '/blog/images/sightkick'

const FIGURES: Record<string, FigureData> = {
  tool: {
    frames: [
      {
        src: `${IMG}/tool-04-promo-before.png`,
        alt: 'Checkout Review step, promo code field empty, total $23.65',
        caption: 'Before: apply_promo(code="BURRITO20") has not run yet.',
      },
      {
        src: `${IMG}/tool-05-promo-after.png`,
        alt: 'Checkout Review step, BURRITO20 applied, total $18.92',
        caption:
          'After: one call filled the code, clicked Apply, and confirmed the discount landed.',
        breadcrumb:
          'guidance → read_order_total: "read the discounted total while it still exists — it doesn\'t survive to the confirmation"',
      },
      {
        src: `${IMG}/tool-06-promo-detail.png`,
        alt: 'Close-up of the order totals showing the promo line and new total',
        caption:
          'The tool\'s own return value: value: "Total: $18.92" — the same number now on screen.',
      },
    ],
  },
  journey: {
    frames: [
      {
        src: `${IMG}/tool-01-menu.png`,
        alt: 'Burrito Co. menu with five items',
        caption: 'read_menu — start of the purchase journey.',
        breadcrumb:
          'guidance → open_item: "customizations only exist on an item\'s own detail page"',
      },
      {
        src: `${IMG}/journey-02-item.png`,
        alt: 'Classic Burrito detail page, steak selected, quantity 2',
        caption: 'customize_item("protein", "steak") + increase_item_quantity.',
        breadcrumb:
          'guidance → add_item_to_cart: "commit the item — this lands you on the cart, not back on the menu"',
      },
      {
        src: `${IMG}/journey-03-cart.png`,
        alt: 'Cart with one line: 2x Classic Burrito, $21.90',
        caption: 'add_item_to_cart — the app navigates straight to the cart.',
        breadcrumb:
          'guidance → go_to_checkout: "only reachable from the cart, and only when the cart is non-empty"',
      },
      {
        src: `${IMG}/journey-04-checkout-delivery.png`,
        alt: 'Checkout Delivery step with address fields filled',
        caption: 'go_to_checkout — lands on step 1 of 3, Delivery.',
        breadcrumb:
          'guidance → submit_delivery_address: "the Delivery step gates the wizard; this advances it to Payment"',
      },
      {
        src: `${IMG}/journey-05-checkout-payment.png`,
        alt: 'Checkout Payment step with card fields filled',
        caption: 'submit_delivery_address — same route, now on step 2, Payment.',
        breadcrumb:
          'guidance → submit_payment_details: "pass a real 16-digit card — the pre-filled masked one is rejected"',
      },
      {
        src: `${IMG}/journey-06-checkout-review.png`,
        alt: 'Checkout Review step before the promo code',
        caption:
          'submit_payment_details — step 3, Review. Still no navigation: same URL as Delivery.',
        breadcrumb:
          'guidance → apply_promo: "BURRITO20 is only applyable on the Review step, before ordering"',
      },
      {
        src: `${IMG}/journey-07-confirmation.png`,
        alt: 'Order confirmation screen showing an order id and the full (non-discounted) total',
        caption:
          'place_order — order confirmed. Notice the total: $23.65, not $18.92 — the promo never reaches this screen.',
        breadcrumb:
          'guidance → read_order_id: "read the generated order id back as proof the order landed"',
      },
    ],
  },
}

export default function SightkickFrames({ figure = 'tool' }: { figure?: string }) {
  const data = FIGURES[figure] ?? FIGURES.tool
  const [i, setI] = useState(0)
  const frame = data.frames[i]
  const last = data.frames.length - 1

  return (
    <figure className="sk-frames">
      <div className="sk-frames__frame">
        <img src={frame.src} alt={frame.alt} />
      </div>
      <div className="sk-frames__controls">
        <button
          type="button"
          className="sk-frames__nav"
          onClick={() => setI((n) => Math.max(0, n - 1))}
          disabled={i === 0}
          aria-label="Previous frame"
        >
          ←
        </button>
        <span className="sk-frames__count">
          {i + 1} / {data.frames.length}
        </span>
        <button
          type="button"
          className="sk-frames__nav"
          onClick={() => setI((n) => Math.min(last, n + 1))}
          disabled={i === last}
          aria-label="Next frame"
        >
          →
        </button>
      </div>
      <figcaption>
        {frame.caption}
        {frame.breadcrumb && <div className="sk-frames__breadcrumb">{frame.breadcrumb}</div>}
      </figcaption>
    </figure>
  )
}
