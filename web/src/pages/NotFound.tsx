import { Link } from 'react-router'
import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import { NOT_FOUND_TITLE, NOT_FOUND_DESCRIPTION } from '../../scripts/lib/site'

export default function NotFound() {
  return (
    <>
      <Seo title={NOT_FOUND_TITLE} description={NOT_FOUND_DESCRIPTION} />
      <Navigation />
      <main className="not-found" data-component="NotFound">
        <div className="container">
          <div className="section-label">404</div>
          <h1>Not on the map</h1>
          <p className="section-desc">
            This page doesn&rsquo;t exist — check the URL, or head back to one that does.
          </p>
          <div className="not-found__actions">
            <Link to="/" className="btn-primary">Back to home</Link>
            <Link to="/blog" className="btn-secondary">Read the blog</Link>
          </div>
        </div>
      </main>
      <Footer />
    </>
  )
}
