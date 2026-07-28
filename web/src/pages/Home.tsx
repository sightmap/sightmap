import Navigation from '@/components/Navigation'
import Hero from '@/components/Hero'
import PitchSection from '@/components/PitchSection'
import SpecSection from '@/components/SpecSection'
import AgentSection from '@/components/AgentSection'
import CTASection from '@/components/CTASection'
import Footer from '@/components/Footer'
import Seo from '@/components/Seo'
import { HOME_TITLE, SITE_DESCRIPTION } from '../../scripts/lib/site'

export default function Home() {
  return (
    <>
      <Seo title={HOME_TITLE} description={SITE_DESCRIPTION} />
      <Navigation />
      <Hero />
      <PitchSection />
      <SpecSection />
      <AgentSection />
      <CTASection />
      <Footer />
    </>
  )
}
