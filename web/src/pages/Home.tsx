import Navigation from '@/components/Navigation'
import Hero from '@/components/Hero'
import PitchSection from '@/components/PitchSection'
import SpecSection from '@/components/SpecSection'
import MemorySection from '@/components/MemorySection'
import GetStarted from '@/components/GetStarted'
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
      <MemorySection />
      <GetStarted />
      <Footer />
    </>
  )
}
