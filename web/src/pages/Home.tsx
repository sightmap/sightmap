import Navigation from '@/components/Navigation'
import Hero from '@/components/Hero'
import PitchSection from '@/components/PitchSection'
import SpecSection from '@/components/SpecSection'
import AgentSection from '@/components/AgentSection'
import CTASection from '@/components/CTASection'
import Footer from '@/components/Footer'

export default function Home() {
  return (
    <>
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
