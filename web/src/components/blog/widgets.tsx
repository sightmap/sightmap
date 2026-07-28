import parse, { Element, type HTMLReactParserOptions } from 'html-react-parser'
import SightmapSnapshot from './SightmapSnapshot'
import SkillVsSightmap from './SkillVsSightmap'

// Interactive figures are authored in markdown as
//
//   <div data-widget="sightmap-snapshot" data-figure="checkout">
//     ...static fallback markup...
//   </div>
//
// `marked` passes that through untouched, so the fallback is what the
// prerender and any non-JS reader sees. Here the marker is swapped for the
// live component.
const WIDGETS: Record<string, (attribs: Record<string, string>) => React.ReactElement> = {
  'sightmap-snapshot': (a) => <SightmapSnapshot figure={a['data-figure']} />,
  'skill-vs-sightmap': () => <SkillVsSightmap />,
}

export const parserOptions: HTMLReactParserOptions = {
  replace: (domNode) => {
    if (!(domNode instanceof Element)) return
    if (domNode.name !== 'div') return
    const widget = domNode.attribs?.['data-widget']
    if (!widget) return
    const render = WIDGETS[widget]
    if (!render) return
    return render(domNode.attribs)
  },
}

export function renderPostBody(html: string) {
  return parse(html, parserOptions)
}
