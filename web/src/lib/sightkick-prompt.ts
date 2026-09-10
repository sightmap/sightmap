// The one-click prompt for adding a WebMCP tool layer with sightmap +
// sightkick. Written to be pasted whole into a coding agent, so it carries
// the install, the skills, and the shape of the job.
//
// Browser-safe (no node: imports): src/pages/Sightkick.tsx renders it, and
// scripts/lib/sightkick-starter.ts personalises it for an Atlas scan report
// so a submitter whose site exposed no tools leaves with a prompt that
// already names their URL and the pages the scan looked at.
export const SIGHTKICK_INSTALL = 'npm install -g @sightmap/sightkick'

// A URL from a scan is the scanned site's own (its redirect target), so it is
// quoted as a shell word; the placeholder stays bare for a reader to replace.
const shellWord = (s: string): string => (s === '<APP_URL>' ? s : `'${s.replace(/'/g, "'\\''")}'`)

export function sightkickAgentPrompt(appUrl = '<APP_URL>', pagesHint = ''): string {
  const pages = pagesHint ? `\n   Start with: ${pagesHint}` : ''
  return `Build a WebMCP tool layer for this app with sightmap + sightkick.

1. npm install -g @sightmap/sightmap @sightmap/sightkick
2. sightmap skills install
   Read sightmap-authoring, sightmap-browser, sightkick-authoring and
   sightkick-debug before you start. They are the source of truth.
3. Start a session against the running app:
   sightmap browser start --url ${shellWord(appUrl)}
4. Follow sightmap-authoring to map the 1-3 pages the tools need. Verify every
   selector with sel-probe before it goes into YAML, get each page to 0
   orphaned nodes, and run sightmap capture on each view.${pages}
5. Follow sightkick-authoring to write .sightkick/tools.yaml. Include at least
   one read tool. Declare a journey so results carry guidance.
6. sightkick build . --verify -o tools.ir.json
7. sightkick browser .            # starts the session, persist-injects the tools
   sightmap browser mcp list      # confirm they registered
   sightkick call . <tool> --param k=v --via cli
   sightkick call . <tool> --param k=v --via webmcp
8. Report what you built, the JSON each tool returned, and anything that failed
   with its actual error text.
9. Get listed. Once sightmap browser mcp list confirms the tools on the
   deployed URL, submit the site to the Sightmap Atlas:
   POST https://sightmap.org/api/atlas/submit
   {"url": "<DEPLOYED_URL>", "email": "<OWNER_EMAIL>", "owner": true, "sightkick": true}
   Or open https://sightmap.org/atlas in a WebMCP-capable session and call its
   submit_site tool. A maintainer reviews every listing before it appears.`
}

export const AGENT_PROMPT = sightkickAgentPrompt()
