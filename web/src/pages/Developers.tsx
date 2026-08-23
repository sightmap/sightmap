import Navigation from '@/components/Navigation'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import { DEVELOPERS_DESCRIPTION, DEVELOPERS_TITLE } from '../../scripts/lib/site'

const RESOURCES: { href: string; label: string; detail: string; external?: boolean }[] = [
  {
    href: '/openapi.json',
    label: 'OpenAPI specification',
    detail: 'Machine-readable description of the Sightmap HTTP API at /openapi.json.',
  },
  {
    href: '/api/openapi.yaml',
    label: 'OpenAPI (YAML)',
    detail: 'The same specification at /api/openapi.yaml.',
  },
  {
    href: '/api/atlas',
    label: 'Atlas HTTP API',
    detail: 'GET /api/atlas lists published sightmaps. GET /api/atlas/{slug} returns one entry.',
  },
  {
    href: 'https://docs.sightmap.org',
    label: 'Sightmap documentation',
    detail: 'Guides, CLI reference, and the schema reference.',
    external: true,
  },
  {
    href: 'https://docs.sightmap.org/start/quickstart',
    label: 'Quickstart',
    detail: 'Install the Sightmap CLI and author a first corpus.',
    external: true,
  },
  {
    href: 'https://github.com/sightmap/sightmap',
    label: 'GitHub repository',
    detail: 'Spec, reference implementation, and agent skills.',
    external: true,
  },
  {
    href: 'https://www.npmjs.com/package/@sightmap/sightmap',
    label: 'CLI on npm',
    detail: 'npm install -g @sightmap/sightmap — then sightmap skills install.',
    external: true,
  },
  {
    href: '/llms.txt',
    label: 'llms.txt',
    detail: 'Published site index for agents, including every Atlas entry.',
  },
]

export default function Developers() {
  return (
    <>
      <Seo title={DEVELOPERS_TITLE} description={DEVELOPERS_DESCRIPTION} />
      <Navigation />
      <main className="developers" data-component="Developers">
        <div className="container">
          <div className="developers__header">
            <div className="section-label">Developers</div>
            <h1>Sightmap developer resources</h1>
            <p className="section-desc">{DEVELOPERS_DESCRIPTION}</p>
            <p className="developers__auth">
              The public HTTP API is read-only and requires no authentication. Errors
              return JSON with <code>error.code</code>, <code>error.message</code>, and <code>error.hint</code>.
            </p>
          </div>
          <div className="developers__list">
            {RESOURCES.map((r) => (
              <a
                key={r.href}
                href={r.href}
                className="developers__item"
                {...(r.external ? { target: '_blank', rel: 'noreferrer' } : {})}
              >
                <div className="developers__item-title">{r.label}</div>
                <p className="developers__item-detail">{r.detail}</p>
                <code className="developers__item-href">{r.href}</code>
              </a>
            ))}
          </div>
        </div>
      </main>
      <Footer />
    </>
  )
}
