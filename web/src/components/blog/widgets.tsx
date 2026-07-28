import parse, { Element, type HTMLReactParserOptions } from 'html-react-parser'
import CodeBlock from './CodeBlock'
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

// Recursively collect the raw text of a parsed <code> element, so a fenced
// code block's contents can be handed to CodeBlock as a plain string.
function extractText(node: Element): string {
  return node.children
    .map((child) => {
      if (child.type === 'text') return (child as unknown as { data: string }).data
      if (child instanceof Element) return extractText(child)
      return ''
    })
    .join('')
}

export const parserOptions: HTMLReactParserOptions = {
  replace: (domNode) => {
    if (!(domNode instanceof Element)) return
    if (domNode.name === 'div') {
      const widget = domNode.attribs?.['data-widget']
      if (!widget) return
      const render = WIDGETS[widget]
      if (!render) return
      return render(domNode.attribs)
    }
    // Fenced code blocks: `marked` renders ```lang blocks as
    // <pre><code class="language-lang">...</code></pre>. Swap that for the
    // interactive CodeBlock (syntax highlighting, line numbers, copy button)
    // both during server prerendering and in the browser, same as the
    // data-widget markers above.
    if (domNode.name === 'pre') {
      const codeEl = domNode.children.find(
        (child): child is Element => child instanceof Element && child.name === 'code'
      )
      if (!codeEl) return
      const code = extractText(codeEl).replace(/\n$/, '')
      const language = /language-([\w-]+)/.exec(codeEl.attribs.class ?? '')?.[1]
      return <CodeBlock code={code} language={language} />
    }
  },
}

export function renderPostBody(html: string) {
  return parse(html, parserOptions)
}
