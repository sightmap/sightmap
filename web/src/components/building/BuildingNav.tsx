import Logo from '@/components/Logo'

// Page-local header. Not a <nav> element on purpose: src/index.css styles the
// bare `nav` tag as the cream site header, and this page swaps to a dark
// treatment at nightfall.
export default function BuildingNav() {
  return (
    <header className="bld-nav" data-component="BuildingNav">
      <a href="/" className="bld-nav__logo" aria-label="sightmap home">
        <Logo />
      </a>
      <span className="bld-nav__title">The Building</span>
      <div className="bld-nav__links" role="navigation" aria-label="Site">
        <a href="/">Home</a>
        <a href="https://docs.sightmap.org">Docs</a>
        <a href="https://github.com/sightmap/sightmap" target="_blank" rel="noreferrer">
          GitHub
        </a>
      </div>
    </header>
  )
}
