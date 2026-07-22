export default function PitchSection() {
  return (
    <section className="pitch" data-component="PitchSection">
      <div className="container">
        <div className="section-label">The Problem</div>
        <p className="pitch-body">
          Every agent that works on your app re-learns it from scratch. It greps source, guesses which component renders which screen, misses the quirks no source file records, and moves on <em>without leaving anything behind</em>.
        </p>
        <p className="pitch-kicker">
          A sightmap is where what agents learn stays.
        </p>
      </div>
    </section>
  )
}
