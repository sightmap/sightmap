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
  return `Make this app usable by agents with sightmap + sightkick.

1. npm install -g @sightmap/sightmap @sightmap/sightkick
2. sightmap skills install
   Read sightmap-authoring, sightmap-browser, sightkick-authoring and
   sightkick-debug before you start. They are the source of truth.
3. Start a session against the running app:
   sightmap browser start --url ${shellWord(appUrl)}
4. Follow sightmap-authoring to map the 1-3 pages the tools need. Verify every
   selector with sel-probe before it goes into YAML, get each page to 0
   orphaned nodes, and run sightmap capture on each view.${pages}
5. Follow sightkick-authoring to write .sightkick/tools.yaml: the 3-6 things a
   visitor comes here to do, as tools with typed params. Include at least one
   read tool. Declare a journey so results carry guidance.
6. sightkick build . --verify -o tools.ir.json
7. sightkick browser .            # starts the session, persist-injects the tools
   sightmap browser mcp list      # confirm they registered
   sightkick call . <tool> --param k=v --via cli
   sightkick call . <tool> --param k=v --via webmcp
8. Ship the tools with the app: sightkick runtime -o <public dir>/sightkick-runtime.js,
   copy tools.ir.json next to it, and load both on the mapped pages (a <script src>
   for the runtime, then window.__sightkick.load(ir)).
9. Generate a claim token: openssl rand -hex 16. Write <public dir>/webmcp.txt: the
   app URL on the first line, then one line per tool, "name — description", then one
   comment line, "# sightmap-claim: <TOKEN>". Lines starting with # are comments and
   readers ignore them. Deploy, then confirm on the deployed URL:
   sightmap browser start --url <DEPLOYED_URL> && sightmap browser mcp list
   The app is now usable by any agent that opens it. Nothing else is required.
10. Report: the tools you built and the JSON each returned, the webmcp.txt URL and
   the claim token, and anything that failed with its actual error text.
11. Optional, only if the owner wants the site shown in the Sightmap Atlas:
   POST https://sightmap.org/api/atlas/submit with
   {"url": "<DEPLOYED_URL>", "email": "<OWNER_EMAIL>", "owner": true, "sightkick": true, "claim": "<TOKEN>"}
   or open https://sightmap.org/atlas in a WebMCP-capable session and call its
   submit_site tool. A submission that carries the claim answers with a card URL,
   https://sightmap.org/try/<host>, which the owner can share immediately. The Atlas
   listing itself appears only after a maintainer reviews it; the page lands at
   https://sightmap.org/atlas/<host with dots as dashes>. Add the submission id and
   the card URL to the report.`
}

export const AGENT_PROMPT = sightkickAgentPrompt()
