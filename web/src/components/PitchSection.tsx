export default function PitchSection() {
  return (
    <section className="pitch" data-component="PitchSection">
      <div className="container">
        <div className="section-label">The Problem</div>
        <p className="pitch-body">
          Every agent working on your app starts from scratch. It greps source, guesses which components render each screen, misses the runtime quirks that no source file records, and moves on <em>without leaving anything behind</em>.
        </p>
        <p className="pitch-kicker">
          Agents record what they learn in a sightmap for the next agent to use.
        </p>
      </div>
    </section>
  )
}
