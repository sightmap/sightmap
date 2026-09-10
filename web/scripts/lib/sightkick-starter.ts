// What a submitter takes away from a scan besides the listing: a Sightkick
// starter kit generated from the report. Pure string builders, so the build
// can bake them into the manifest and the page needs no logic of its own.
//
// Two shapes, by what the scan found:
//
//   - Tools found → a verification transcript. The tools already exist, so
//     the useful next step is a replayable check that they still behave as
//     listed: `sightmap browser mcp list` to confirm the surface and one
//     `mcp call` per read tool with a placeholder argument. This is the seed
//     of a scenario plan (see sightkick docs/scenario-testing.md) — deterministic,
//     re-runnable, and the thing a rescan can be diffed against.
//   - No tools → a `.sightkick/tools.yaml` skeleton drafted from the forms
//     and links the scan saw, plus the agent prompt personalised with the
//     URL. The skeleton names components the corpus does not have yet; the
//     comments say so, because a tool layer compiles only against a corpus.
//
// Everything interpolated here comes from the scanned site — tool names,
// input-schema property keys, form field names, the URL it settled on — and
// the page ships the result under a Copy button, so it is treated as hostile
// input. Identifiers are allowlisted by shape and anything else is left out
// (it is still in the listing's tool table, which is data, not a command);
// the one free-form value, the URL, is single-quoted for the shell.
import type { ScanReport, ScanTool, SightkickStarter } from '../../src/types/directory'
import { sightkickAgentPrompt } from '../../src/lib/sightkick-prompt'

export type { SightkickStarter }

/** A tool name safe to paste into a command line: snake_case, nothing else. */
const TOOL_NAME = /^[a-z][a-z0-9_]*$/
/** An input-schema property key safe as a `--param` name. */
const PARAM_KEY = /^[A-Za-z_][A-Za-z0-9_]*$/
/** A form field name safe inside a generated component name and YAML key. */
const FIELD_NAME = /^[A-Za-z0-9_]+$/

/**
 * POSIX single-quoting: inside `'…'` every byte is literal, so the only thing
 * to handle is the quote itself — end the string, escape one quote, start a
 * new one (`'\''`). Newlines, `$`, backticks and `;` all lose their meaning.
 */
export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function placeholderArgs(tool: ScanTool): string {
  const schema = tool.inputSchema as { properties?: Record<string, { type?: string }>; required?: string[] } | null
  const props = schema?.properties ?? {}
  const keys = Object.keys(props).filter((k) => PARAM_KEY.test(k))
  if (keys.length === 0) return ''
  const required = new Set(schema?.required ?? [])
  const chosen = keys.filter((k) => required.has(k))
  const use = (chosen.length > 0 ? chosen : keys).slice(0, 3)
  return use
    .map((k) => {
      const t = props[k]?.type
      const v = t === 'number' || t === 'integer' ? '1' : t === 'boolean' ? 'true' : `"<${k}>"`
      return ` --param ${k}=${v}`
    })
    .join('')
}

function toolFromForm(form: ScanReport['hints']['forms'][number], index: number): string {
  // A field name becomes a YAML key, a `{{template}}` reference and half a
  // component name, so anything but a plain identifier is dropped rather than
  // escaped three different ways.
  const fields = form.fields.filter((f) => FIELD_NAME.test(f))
  const name = fields.includes('q') || fields.includes('query') || fields.includes('search') ? 'search' : `submit_form_${index + 1}`
  const params = fields
    .map(
      (f) => `      - name: ${f}
        type: string
        required: ${index === 0 && fields.length === 1 ? 'true' : 'false'}
        description: TODO — what ${f} means to a user.`
    )
    .join('\n')
  const steps = fields.map((f) => `      - fill: { query: Form${index + 1}Field_${f}, value: "{{${f}}}" }`).join('\n')
  return `  - name: ${name}
    description: TODO — one sentence, what the agent gets when it calls this.
    ensure_view: ${index === 0 ? 'Home' : `View${index + 1}`}   # a VIEW name from your .sightmap/ corpus
    params:
${params || '      []'}
    steps:
${steps || '      []'}
      - click: { query: Form${index + 1}Submit }
      - wait_for: { query: Form${index + 1}Result }   # the visible feedback the submit produces
    returns:
      description: TODO — what changed, as a value or a list.`
}

export function sightkickStarter(report: ScanReport): SightkickStarter {
  const pagesHint = report.pages
    .filter((p) => !p.error)
    .map((p) => p.path)
    .slice(0, 3)
    .join(', ')
  const prompt = sightkickAgentPrompt(report.finalUrl, pagesHint)

  if (report.tools.length > 0) {
    const reads = report.tools.filter((t) => t.risk === 'read')
    // A tool name goes onto a command line the submitter is invited to copy,
    // so only plain snake_case names are scripted. The rest are named in the
    // listing's tool table instead, where they are text and not a command.
    const callable = reads.filter((t) => TOOL_NAME.test(t.name))
    const omitted = reads.length - callable.length
    const lines: string[] = []
    if (omitted > 0) {
      lines.push(
        `# ${omitted} read tool(s) are missing here: their names are not snake_case, so`,
        `# they are left out of a copyable transcript. See the tool list above.`
      )
    }
    if (callable.length > 0) {
      for (const t of callable.slice(0, 5)) lines.push(`sightmap browser mcp call ${t.name}${placeholderArgs(t)}`)
    } else if (reads.length === 0) {
      lines.push('# No read-only tool was found; add one (a search or a get) before scripting a check.')
    } else {
      lines.push('# No read-only tool has a name that can be scripted as-is; rename one to snake_case first.')
    }
    const calls = lines.join('\n')
    const code = `# Replayable check for ${report.host}: confirm the surface, then call each
# read-only tool with a known-good argument and compare the JSON to what you
# expect. Run it after every deploy; keep the transcript next to the code.
npm install -g @sightmap/sightmap
sightmap browser start --url ${shellQuote(report.finalUrl)} --headless
sightmap browser mcp list --json          # expect ${report.counts.tools} tool(s), surface: ${report.surface}
${calls}
sightmap browser stop`
    return {
      kind: 'verify',
      title: 'Turn these tools into a replayable check',
      intro:
        'The scan found tools. The next step is a deterministic check that they still behave as listed — a one-file transcript a CI job can rerun, and the seed of a Sightkick scenario plan.',
      code,
      lang: 'sh',
      prompt,
    }
  }

  const forms = report.hints.forms.slice(0, 3)
  const droppedFields = forms.reduce((n, f) => n + f.fields.filter((x) => !FIELD_NAME.test(x)).length, 0)
  const skeleton =
    forms.length > 0
      ? forms.map(toolFromForm).join('\n\n')
      : `  - name: get_page_summary
    description: TODO — read the one thing a visitor comes to this page for.
    ensure_view: Home   # a VIEW name from your .sightmap/ corpus
    returns:
      description: The page's main heading and lead paragraph.
      value: { query: PageHeading, property: text }`
  const dropped =
    droppedFields > 0
      ? `\n# ${droppedFields} form field(s) whose names are not plain identifiers were skipped.`
      : ''
  const code = `# .sightkick/tools.yaml — drafted from the Atlas scan of ${report.host}.
# Every component name below (Form1Field_q, Form1Submit, …) is a placeholder:
# a tool layer compiles only against a .sightmap/ corpus, so map the page
# first (sightmap-authoring), then rename these to the components you declared.${dropped}
version: 1
name: ${report.host.replace(/[^a-z0-9]+/gi, '-')}

tools:
${skeleton}

journeys:
  - name: first_visit
    description: TODO — the one flow a first-time visitor completes.
    steps:
      - ${forms.length > 0 ? (forms[0].fields.includes('q') ? 'search' : 'submit_form_1') : 'get_page_summary'}`
  return {
    kind: 'author',
    title: 'Start a tool layer with Sightkick',
    intro:
      forms.length > 0
        ? `The scan found no WebMCP tools, but it saw ${forms.length} form${forms.length === 1 ? '' : 's'} an agent would want to drive. This skeleton turns them into typed tools; the prompt below hands the whole job to a coding agent.`
        : 'The scan found no WebMCP tools. This skeleton and the prompt below are the shortest path to a first read tool.',
    code,
    lang: 'yaml',
    prompt,
  }
}
