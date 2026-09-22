import { useState } from 'react'

// Frame-sequence stepper: a numbered set of real screenshots captured
// mid-session, stepped one at a time. Three figures: "tool" walks one tool
// call's before/after, "customize" does the same for the item page, and
// "journey" walks a full purchase across all five views.
//
// Each frame separates three things that are easy to confuse when they run
// together in one caption: the call that produced this screen (with the
// params the feature line supplied), what the screen shows, and the
// guidance breadcrumb naming the call to make next. A journey never
// executes anything itself, so that last one is advice, not history.

type Frame = {
  src: string
  alt: string
  // The call this frame is the result of, params included.
  call: string
  caption: string
  // The successor the prior call's guidance pointed at, and the author's
  // reason for it. Advice for the next call, not something that ran.
  next?: { tool: string; reason: string }
}

type FigureData = { frames: Frame[] }

const IMG = '/blog/images/sightkick'

const FIGURES: Record<string, FigureData> = {
  tool: {
    frames: [
      {
        src: `${IMG}/tool-04-promo-before.png`,
        alt: 'Checkout Review step, promo code field empty, total $23.65',
        call: 'apply_promo(code: "BURRITO20")',
        caption: 'Before the call: the promo field is empty and the total is $23.65.',
      },
      {
        src: `${IMG}/tool-05-promo-after.png`,
        alt: 'Checkout Review step, BURRITO20 applied, total $18.92',
        call: 'apply_promo(code: "BURRITO20")',
        caption:
          'After: one call filled the code, clicked Apply, and waited for the discount to land.',
        next: {
          tool: 'read_order_total',
          reason:
            "read the discounted total while it still exists, it doesn't survive to the confirmation",
        },
      },
      {
        src: `${IMG}/tool-06-promo-detail.png`,
        alt: 'Close-up of the order totals showing the promo line and new total',
        call: 'apply_promo → returns',
        caption:
          'The tool\'s own return value: value: "Total: $18.92", read off ReviewTotals.total, the same number now on screen.',
      },
    ],
  },
  customize: {
    frames: [
      {
        src: `${IMG}/tool-02-item-before.png`,
        alt: 'Classic Burrito detail page, PROTEIN group with chicken selected, quantity 1',
        call: 'customize_item(group: "protein", option: "steak")',
        caption: 'Before the call: the PROTEIN group still has its default, chicken, selected.',
      },
      {
        src: `${IMG}/tool-03-item-after.png`,
        alt: 'The same page with steak now selected in the PROTEIN group',
        call: 'customize_item(group: "protein", option: "steak")',
        caption:
          'After: the option came back with a selected class, which is exactly what wait_for watches for. Call it again and the guard matches that same state, so nothing clicks twice.',
      },
    ],
  },
  journey: {
    frames: [
      {
        src: `${IMG}/tool-01-menu.png`,
        alt: 'Burrito Co. menu with five items',
        call: 'read_menu()',
        caption: 'The start of the purchase journey: five menu rows, name and price.',
        next: {
          tool: 'open_item',
          reason: "customizations only exist on an item's own detail page",
        },
      },
      {
        src: `${IMG}/journey-02-item.png`,
        alt: 'Classic Burrito detail page, steak selected, quantity 2',
        call: 'customize_item(group: "protein", option: "steak") → increase_item_quantity()',
        caption: 'Steak selected, quantity bumped to 2. Quantity is set here, not in the cart.',
        next: {
          tool: 'add_item_to_cart',
          reason: 'commit the item, this lands you on the cart, not back on the menu',
        },
      },
      {
        src: `${IMG}/journey-03-cart.png`,
        alt: 'Cart with one line: 2x Classic Burrito, $21.90',
        call: 'add_item_to_cart()',
        caption: 'The tool does not just add, it navigates: the app lands straight on the cart.',
        next: {
          tool: 'go_to_checkout',
          reason: 'only reachable from the cart, and only when the cart is non-empty',
        },
      },
      {
        src: `${IMG}/journey-04-checkout-delivery.png`,
        alt: 'Checkout Delivery step with address fields filled',
        call: 'go_to_checkout()',
        caption: 'Lands on step 1 of 3, Delivery.',
        next: {
          tool: 'submit_delivery_address',
          reason: 'the Delivery step gates the wizard; this advances it to Payment',
        },
      },
      {
        src: `${IMG}/journey-05-checkout-payment.png`,
        alt: 'Checkout Payment step with card fields filled',
        call:
          'submit_delivery_address(street: "123 Main St", city: "Denver", state: "CO", zip: "80203")',
        caption: 'Same route, now on step 2, Payment. The returned value is the active step name.',
        next: {
          tool: 'submit_payment_details',
          reason: 'pass a real 16-digit card, the pre-filled masked one is rejected',
        },
      },
      {
        src: `${IMG}/journey-06-checkout-review.png`,
        alt: 'Checkout Review step before the promo code',
        call: 'submit_payment_details(card_number: "4242 4242 4242 4242", expiry: "09/26")',
        caption: 'Step 3, Review. Still no navigation: the same URL as Delivery.',
        next: {
          tool: 'apply_promo',
          reason: 'BURRITO20 is only applyable on the Review step, before ordering',
        },
      },
      {
        src: `${IMG}/journey-07-confirmation.png`,
        alt: 'Order confirmation screen showing an order id and the full (non-discounted) total',
        call: 'place_order()',
        caption:
          'Order confirmed. Note the total: $23.65, not $18.92. The promo never reaches this screen.',
        next: {
          tool: 'read_order_id',
          reason: 'read the generated order id back as proof the order landed',
        },
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
        <div className="sk-frames__call">
          <span className="sk-frames__label">called</span>
          <code>{frame.call}</code>
        </div>
        <div className="sk-frames__note">{frame.caption}</div>
        {frame.next && (
          <div className="sk-frames__breadcrumb">
            <span className="sk-frames__label">next call</span>
            <code>{frame.next.tool}</code>
            <span className="sk-frames__reason">{frame.next.reason}</span>
          </div>
        )}
      </figcaption>
    </figure>
  )
}
