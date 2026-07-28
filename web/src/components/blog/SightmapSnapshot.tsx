import { useState } from 'react'

// Toggle widget for the sightmaps post: the same accessibility tree, shown
// once as a browser-automation tool would see it raw, and once after a
// sightmap has named everything in it. Same DOM, same pixels: only the
// labels and the attached memory change.

type Mode = 'raw' | 'sightmap'

type FigureData = {
  view: string
  defaultMode?: Mode
  raw: { tokens: string; tree: string }
  sightmap: { tokens: string; tree: string; guide: string[] }
}

const FIGURES: Record<string, FigureData> = {
  checkout: {
    view: '/checkout',
    raw: {
      tokens: 'DOM roles',
      tree: `[generic]
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
    [button] "Continue" [disabled]`,
    },
    sightmap: {
      tokens: 'names + memory',
      tree: `[View: Checkout "/checkout"]
[CheckoutForm]
  [heading] "Checkout"
  [CardFieldset]
    [CardNumberInput] "4242 4242 4242 4242"
    [ExpiryInput] "12 / 28"
    [CvcInput] "123"
  [PromoCodeField]
    [textbox]
    [ApplyPromoButton]
  [OrderTotal] "Total: $42.00"
  [ContinueButton] [disabled]`,
      guide: [
        'CardFieldset validates on blur. Tab or click off a field before submitting, or ContinueButton stays disabled.',
        'src: src/components/checkout/CheckoutForm.tsx',
      ],
    },
  },
  datepicker: {
    view: '/checkout/delivery',
    defaultMode: 'sightmap',
    raw: {
      tokens: 'DOM roles',
      tree: `[generic]
  [textbox] "12/28/2026"
  [generic]
    [generic]
      [button] "Previous month"
      [generic] "December 2026"
      [button] "Next month"
    [gridcell] "1"
    [gridcell] "2"
    [gridcell] "3"
    …24 more gridcells…`,
    },
    sightmap: {
      tokens: 'names + memory',
      tree: `[View: Delivery "/checkout/delivery"]
[DeliveryDatePicker]
  [DateTrigger] "12/28/2026"
  [CalendarDialog]
    [PrevMonthButton]
    [MonthLabel] "December 2026"
    [NextMonthButton]
    [Day] "1"
    [Day] "2"
    [Day] "3"
    …24 more days…`,
      guide: [
        'DateTrigger accepts a typed date (e.g. 12/28/2026). Skip the calendar.',
        'src: src/components/DeliveryDatePicker.tsx',
      ],
    },
  },
}

export default function SightmapSnapshot({ figure = 'checkout' }: { figure?: string }) {
  const data = FIGURES[figure] ?? FIGURES.checkout
  const [mode, setMode] = useState<Mode>(data.defaultMode ?? 'raw')
  const current = data[mode]

  return (
    <figure className="smap-widget">
      <div className="smap-widget__controls" role="tablist" aria-label="Snapshot source">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'raw'}
          className={`smap-widget__btn ${mode === 'raw' ? 'is-active' : ''}`}
          onClick={() => setMode('raw')}
        >
          <span className="smap-widget__btn-name">raw a11y tree</span>
          <span className="smap-widget__btn-tok">{data.raw.tokens}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'sightmap'}
          className={`smap-widget__btn ${mode === 'sightmap' ? 'is-active' : ''}`}
          onClick={() => setMode('sightmap')}
        >
          <span className="smap-widget__btn-name">with a sightmap</span>
          <span className="smap-widget__btn-tok">{data.sightmap.tokens}</span>
        </button>
      </div>
      <div className="smap-widget__tree">
        <pre>
          <code>{current.tree}</code>
        </pre>
      </div>
      {mode === 'sightmap' && (
        <div className="smap-widget__guide">
          <div className="smap-widget__guide-label">[Memory]</div>
          <ul>
            {data.sightmap.guide.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </div>
      )}
      <figcaption>
        Same DOM, same pixels, {data.view}. A sightmap snapshot costs about the same as the raw tree
        per turn, sometimes less, sometimes more. The win is that the next agent reads the names and
        the memory instead of re-deriving them.
      </figcaption>
    </figure>
  )
}
