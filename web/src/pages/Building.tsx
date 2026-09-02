import Seo from '@/components/Seo'
import BuildingExperience from '@/components/building/BuildingExperience'
import { BUILDING_TITLE, BUILDING_DESCRIPTION } from '../../scripts/lib/site'

// The immersive tour. Everything interesting lives in components/building/;
// this page only sets metadata. The WebGL scene is client-only and lazy, so
// the prerender of this route carries the SVG poster plus the full chapter
// text and never loads three.js in Node.
export default function Building() {
  return (
    <>
      <Seo title={BUILDING_TITLE} description={BUILDING_DESCRIPTION} />
      <BuildingExperience />
    </>
  )
}
