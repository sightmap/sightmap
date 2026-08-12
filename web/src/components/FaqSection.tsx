const FAQS = [
  {
    q: 'Do I have to write the sightmap by hand?',
    a: 'No. Point an agent at a running app and it drafts the map for you — you review the diff like any other change.',
  },
  {
    q: 'What happens when the UI changes?',
    a: 'Selectors stop resolving and coverage drops. Both show up in `sightmap lint` and in the coverage tiers, so a stale map is visible rather than silently wrong.',
  },
  {
    q: 'Does this replace a skill or an AGENTS.md?',
    a: 'It sits underneath them. A skill tells an agent how to work; a sightmap tells it what is actually on the screen right now.',
  },
]

export default function FaqSection() {
  return (
    <section className="faq" data-component="FaqSection">
      <div className="container">
        <div className="section-label">Questions</div>
        <h2>The ones people actually ask</h2>
        <div className="faq-list">
          {FAQS.map((item) => (
            <details className="faq-item" key={item.q}>
              <summary className="faq-q">{item.q}</summary>
              <p className="faq-a">{item.a}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  )
}
