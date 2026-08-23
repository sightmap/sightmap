import { useConsent } from './consent/ConsentContext'

export default function Footer() {
  const { openPreferences } = useConsent()
  return (
    <footer data-component="Footer">
      <div className="container">
        <div className="footer-inner">
          <div className="footer-left">
            sightmap · open-source spec by <a href="https://subtext.fullstory.com" target="_blank" rel="noreferrer">Subtext</a>
          </div>
          <div className="footer-links">
            <a href="/atlas">Atlas</a>
            <a href="/blog">Blog</a>
            <a href="/developers">Developers</a>
            <a href="https://docs.sightmap.org">Docs</a>
            <a href="https://docs.sightmap.org/start/quickstart">Quickstart</a>
            <a href="https://docs.sightmap.org/reference/schema">Schema reference</a>
            <a href="https://github.com/sightmap/sightmap" target="_blank" rel="noreferrer">GitHub</a>
            <a href="https://www.npmjs.com/package/@sightmap/sightmap" target="_blank" rel="noreferrer">CLI on npm</a>
            <a href="https://github.com/sightmap/sightmap/blob/main/LICENSE" target="_blank" rel="noreferrer">MIT License</a>
            <button type="button" onClick={openPreferences} data-action="consent-open-preferences" data-fs-element="Sightmap | Consent: Open Preferences">Privacy preferences</button>
          </div>
        </div>
      </div>
    </footer>
  )
}
