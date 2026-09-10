// Classification and static checks over a scanned WebMCP tool set. Pure
// functions, shared by the scanner (scripts/atlas-scan.ts), the review step,
// and the tests — nothing here touches a browser or the filesystem.
//
// The classification is a heuristic and is labelled that way everywhere it is
// shown ("classified by Atlas"). It errs toward the riskier bucket: a tool the
// heuristics cannot place is an `action`, never a `read`, so a maintainer's
// correction only ever relaxes a label.
import type { ScanCheck, ScanCounts, ScanTool, ToolKind } from '../../src/types/directory'

// Money, commitment, or hard-to-reverse effects. Matched as whole words
// against the snake_case-split name and the description.
const SENSITIVE = [
  'pay',
  'payment',
  'purchase',
  'buy',
  'checkout',
  'order',
  'place_order',
  'book',
  'booking',
  'reserve',
  'reservation',
  'subscribe',
  'unsubscribe',
  'cancel',
  'delete',
  'destroy',
  'remove_account',
  'transfer',
  'withdraw',
  'deposit',
  'refund',
  'charge',
  'donate',
  'send',
  'email',
  'message',
  'publish',
  'post',
  'upload',
  'sign',
  'apply',
  'application',
  'invite',
  'wire',
  'bid',
  'tip',
  'redeem',
]

// Reversible state or UI changes.
const ACTION = [
  'add',
  'set',
  'update',
  'edit',
  'toggle',
  'select',
  'open',
  'close',
  'navigate',
  'go',
  'goto',
  'filter',
  'sort',
  'clear',
  'reset',
  'create',
  'remove',
  'save',
  'share',
  'request',
  'suggest',
  'record',
  'login',
  'logout',
  'sign_in',
  'sign_out',
  'switch',
  'change',
  'start',
  'stop',
  'play',
  'pause',
  'like',
  'follow',
  'unfollow',
  'submit',
  'nominate',
  'report',
  'load',
  'surprise',
]

// Read-only information.
const READ = [
  'get',
  'list',
  'search',
  'find',
  'lookup',
  'fetch',
  'read',
  'show',
  'about',
  'describe',
  'count',
  'check',
  'summarize',
  'summary',
  'view',
  'explain',
  'help',
  'is',
  'has',
  'query',
  'browse',
  'compare',
  'status',
  'info',
  'details',
  'current',
  'recommend',
]

// Read verbs strong enough to override a sensitive noun later in the name
// (`get_order_status` reads an order, it does not place one).
const READ_LEADS = ['get', 'list', 'search', 'find', 'lookup', 'check', 'is', 'has']

const words = (s: string): string[] =>
  s
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter(Boolean)

function hasAny(haystack: string[], needles: string[]): string | null {
  const set = new Set(haystack)
  for (const n of needles) {
    if (n.includes('_')) {
      // A two-word needle must appear as consecutive words.
      const [a, b] = n.split('_')
      for (let i = 0; i + 1 < haystack.length; i++) {
        if (haystack[i] === a && haystack[i + 1] === b) return n
      }
      continue
    }
    if (set.has(n)) return n
  }
  return null
}

export interface RiskVerdict {
  risk: ToolKind
  reason: string
}

/**
 * Places one tool on the read / action / sensitive ladder from its name and
 * description. The name is decisive when it starts with a read verb and the
 * description carries no sensitive word: `get_order_status` is a read even
 * though `order` is a sensitive word.
 */
export function classifyTool(name: string, description: string): RiskVerdict {
  const nameWords = words(name)
  const descWords = words(description)

  const sensitiveInName = hasAny(nameWords, SENSITIVE)
  const readLead = nameWords[0] && READ.includes(nameWords[0]) ? nameWords[0] : null

  if (sensitiveInName && !(readLead && READ_LEADS.includes(readLead))) {
    return { risk: 'sensitive', reason: `name contains "${sensitiveInName}"` }
  }

  if (readLead) {
    return { risk: 'read', reason: `name starts with "${readLead}"` }
  }

  // The name is the stronger signal: `set_sort` with "change the sort order"
  // in its description is an action, not an order placement.
  const actionInName = hasAny(nameWords, ACTION)
  if (actionInName) return { risk: 'action', reason: `name contains "${actionInName}"` }

  const sensitiveInDesc = hasAny(descWords, SENSITIVE)
  if (sensitiveInDesc && !hasAny(nameWords, READ)) {
    return { risk: 'sensitive', reason: `description mentions "${sensitiveInDesc}"` }
  }

  const readInName = hasAny(nameWords, READ)
  if (readInName) return { risk: 'read', reason: `name contains "${readInName}"` }

  const actionInDesc = hasAny(descWords, ACTION)
  if (actionInDesc) return { risk: 'action', reason: `description mentions "${actionInDesc}"` }

  const readInDesc = hasAny(descWords, READ)
  if (readInDesc) return { risk: 'read', reason: `description mentions "${readInDesc}"` }

  return { risk: 'action', reason: 'unclassified — defaults to action until reviewed' }
}

// Phrases that have no business in a tool description shown to an agent.
// A description is instructions to a model, so this is where an injection
// would live; matching is deliberately broad and every hit is a warning for a
// human, not an automatic rejection.
const SUSPICIOUS = [
  /ignore (all|any|the|previous|prior|above)/i,
  /disregard (all|any|the|previous|prior|above)/i,
  /system prompt/i,
  /you must (always|never|now)/i,
  /always call/i,
  /do not (tell|inform|mention)/i,
  /without (asking|telling|confirming)/i,
  /(api|secret|access) ?(key|token)/i,
  /<script/i,
  /javascript:/i,
  /exfiltrat/i,
  /credit card number/i,
  /password/i,
]

export const MAX_DESCRIPTION_CHARS = 1500

export function hasInputSchema(schema: unknown): boolean {
  return schema !== null && typeof schema === 'object'
}

function isObjectSchema(schema: unknown): boolean {
  if (!hasInputSchema(schema)) return false
  const s = schema as Record<string, unknown>
  return s.type === 'object' || typeof s.properties === 'object'
}

/** Warnings for one tool's metadata, empty when nothing looks off. */
export function toolWarnings(tool: Pick<ScanTool, 'name' | 'description' | 'inputSchema'>): string[] {
  const out: string[] = []
  if (!tool.name.trim()) out.push('tool has no name')
  else if (!/^[a-z][a-z0-9_]*$/.test(tool.name)) out.push('name is not snake_case')
  if (!tool.description.trim()) out.push('no description')
  else if (tool.description.trim().length < 12) out.push('description is very short')
  if (tool.description.length > MAX_DESCRIPTION_CHARS) {
    out.push(`description is over ${MAX_DESCRIPTION_CHARS} characters`)
  }
  for (const re of SUSPICIOUS) {
    if (re.test(tool.description) || re.test(tool.name)) {
      out.push(`metadata matches ${re.source}`)
      break
    }
  }
  if (!hasInputSchema(tool.inputSchema)) out.push('no input schema')
  else if (!isObjectSchema(tool.inputSchema)) out.push('input schema is not an object schema')
  return out
}

/**
 * The factual indicators a listing shows instead of a grade: each is a plain
 * yes/no with the number behind it, so a visitor can disagree with the
 * weighting because there is none.
 */
export function runChecks(tools: ScanTool[], pages: { path: string; tools: string[] }[]): ScanCheck[] {
  const n = tools.length
  const named = tools.filter((t) => t.name.trim()).length
  const described = tools.filter((t) => t.description.trim().length >= 12).length
  const schemas = tools.filter((t) => hasInputSchema(t.inputSchema)).length
  const snake = tools.filter((t) => /^[a-z][a-z0-9_]*$/.test(t.name)).length
  const names = new Set(tools.map((t) => t.name))
  const suspicious = tools.filter((t) => t.warnings.some((w) => w.startsWith('metadata matches')))
  const toolSets = new Set(pages.map((p) => [...p.tools].sort().join(' ')))
  const routeScoped = pages.length > 1 && toolSets.size > 1

  const ratio = (k: number) => `${k}/${n}`
  return [
    { id: 'named', label: 'Every tool has a name', ok: n > 0 && named === n, detail: ratio(named) },
    {
      id: 'described',
      label: 'Every tool has a description',
      ok: n > 0 && described === n,
      detail: ratio(described),
    },
    {
      id: 'input-schema',
      label: 'Every tool has an input schema',
      ok: n > 0 && schemas === n,
      detail: ratio(schemas),
    },
    { id: 'snake-case', label: 'Tool names are snake_case', ok: n > 0 && snake === n, detail: ratio(snake) },
    { id: 'unique', label: 'Tool names are unique', ok: n > 0 && names.size === n, detail: `${names.size} distinct` },
    {
      id: 'no-suspicious',
      label: 'No suspicious wording in tool metadata',
      ok: suspicious.length === 0,
      detail: suspicious.length === 0 ? 'none flagged' : suspicious.map((t) => t.name).join(', '),
    },
    {
      id: 'route-scoped',
      label: 'Tool set changes with the page',
      ok: routeScoped,
      detail:
        pages.length < 2
          ? 'only one page checked'
          : routeScoped
            ? `${toolSets.size} distinct tool sets across ${pages.length} pages`
            : `same tools on all ${pages.length} pages`,
    },
  ]
}

export function countTools(tools: ScanTool[], pages: unknown[]): ScanCounts {
  return {
    tools: tools.length,
    pages: pages.length,
    read: tools.filter((t) => t.risk === 'read').length,
    action: tools.filter((t) => t.risk === 'action').length,
    sensitive: tools.filter((t) => t.risk === 'sensitive').length,
    described: tools.filter((t) => t.description.trim().length >= 12).length,
    withInputSchema: tools.filter((t) => hasInputSchema(t.inputSchema)).length,
    declarative: tools.filter((t) => t.impl === 'declarative').length,
  }
}
