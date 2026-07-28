// Skill vs. sightmap, the two ways to give an agent context. Left: a skill
// dumps the whole rule library up front, everything lit at once. Right: a
// sightmap discloses only the component in view plus its recall/context.
// Theme-aware inline SVG, one looping pass that lights the right side's
// spotlight and chips. No character art; the fun is in the composition.

export default function SkillVsSightmap() {
  const wall = Array.from({ length: 11 })

  return (
    <figure className="smap-svs code-dark">
      <div className="smap-svs__title">
        <span className="smap-svs__title-dim">A skill</span>
        <span className="smap-svs__title-vs"> vs. </span>
        <span className="smap-svs__title-hi">a sightmap</span>
      </div>

      <div className="smap-svs__panels">
        <div className="smap-svs__panel">
          <div className="smap-svs__head">
            <span className="smap-svs__label">Skill dump</span>
            <span className="smap-svs__count">~2,300 tokens</span>
          </div>
          <div className="smap-svs__screen">
            <div className="smap-svs__screen-tab">STATIC RULE LIBRARY</div>
            <div className="smap-svs__wall">
              {wall.map((_, i) => (
                <span
                  key={i}
                  className="smap-svs__wall-line"
                  style={{ width: `${60 + ((i * 37) % 38)}%` }}
                />
              ))}
            </div>
          </div>
          <p className="smap-svs__cap">The whole map, loaded every turn. Grows with the app.</p>
        </div>

        <div className="smap-svs__arrow" aria-hidden="true">
          <span className="smap-svs__arrow-label">progressive disclosure</span>
          <svg viewBox="0 0 48 16" className="smap-svs__arrow-svg">
            <path
              d="M2 8 H40 M34 3 L41 8 L34 13"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>

        <div className="smap-svs__panel">
          <div className="smap-svs__head">
            <span className="smap-svs__label">Sightmap</span>
            <span className="smap-svs__count">~500 tokens</span>
          </div>
          <div className="smap-svs__screen">
            <div className="smap-svs__chips">
              <span className="smap-svs__chip">Recall</span>
              <span className="smap-svs__chip">Components</span>
              <span className="smap-svs__chip">Context</span>
            </div>
            <div className="smap-svs__rows">
              <span className="smap-svs__row" />
              <span className="smap-svs__row smap-svs__row--hi">ContinueButton</span>
              <span className="smap-svs__row" />
            </div>
          </div>
          <p className="smap-svs__cap">Only the active view. Bounded, whatever the app's size.</p>
        </div>
      </div>

      <p className="smap-svs__foot">
        Rough token counts from the burrito demo corpus: the full map versus a single view.
      </p>
    </figure>
  )
}
