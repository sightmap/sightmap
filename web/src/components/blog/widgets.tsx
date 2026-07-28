import parse, { Element, type HTMLReactParserOptions } from 'html-react-parser'
import SightmapSnapshot from './SightmapSnapshot'
import SkillVsSightmap from './SkillVsSightmap'

// Interactive figures are authored in markdown as
//
//   <div data-widget="sightmap-snapshot" data-figure="checkout">
//     ...anything here is ignored...
//   </div>
//
// `marked` passes that div through untouched. `replace` below then swaps the
// whole node for the live component, both during server prerendering
// (renderToString) and in the browser, so crawlers and non-JS readers get
// the real component output. Any children authored inside a marker are
// discarded and never render anywhere.
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
