#!/usr/bin/env node
// Live verification of the generated github example against real github.com.
//
// Not part of the jest suite (CI is hermetic; this needs network). Run it
// manually after regenerating, from the repo root:
//
//   node webmcp/examples/github/verify-live.js [owner repo pr_number]
//
// It executes every tool through the same compiled IR the bundle embeds, in
// jsdom, with fetch() backed by curl (which honors HTTPS_PROXY / CA bundles
// in proxied environments where Node's fetch does not).

const path = require("path");
const { execFileSync } = require("child_process");

const ROOT = path.join(__dirname, "..", "..", "..");
const [owner = "sightmap", repo = "sightmap", prNumber = "281"] =
  process.argv.slice(2);

async function main() {
  // jsdom provides DOMParser for fetch-mode flows.
  const { JSDOM } = require(
    require.resolve("jsdom", { paths: [path.join(ROOT, "node_modules")] }),
  );
  const dom = new JSDOM("<!doctype html><html><body></body></html>", {
    url: "https://github.com/",
  });
  global.window = dom.window;
  global.document = dom.window.document;
  global.DOMParser = dom.window.DOMParser;
  global.location = dom.window.location;

  global.fetch = async (url, init = {}) => {
    const args = ["-sS", "--max-time", "30", "-w", "\n%{http_code}"];
    for (const [k, v] of Object.entries(init.headers || {}))
      args.push("-H", `${k}: ${v}`);
    if (init.method && init.method !== "GET") args.push("-X", init.method);
    if (init.body) args.push("--data-binary", init.body);
    args.push(url);
    const raw = execFileSync("curl", args, {
      encoding: "utf8",
      maxBuffer: 32 * 1024 * 1024,
    });
    const cut = raw.lastIndexOf("\n");
    const body = raw.slice(0, cut);
    const status = parseInt(raw.slice(cut + 1), 10);
    return {
      status,
      text: async () => body,
      headers: { forEach: () => {} },
    };
  };

  const manifestPath = path.join(__dirname, "webmcp.tools.yaml");
  const ir = JSON.parse(
    execFileSync("go", ["run", "./webmcp/internal/dumpir", manifestPath], {
      cwd: path.join(ROOT, "go"),
      encoding: "utf8",
    }),
  );
  const rt = require("../../src/runtime/runtime");

  const CALLS = {
    list_pull_requests: { owner, repo },
    list_issues: { owner, repo },
    list_releases: { owner, repo },
    get_repo_files: { owner, repo },
    get_pr_diffstat: { owner, repo, number: parseInt(prNumber, 10) },
  };

  let failed = 0;
  for (const tool of ir.tools) {
    const args = CALLS[tool.name];
    process.stdout.write(`\n=== ${tool.name}(${JSON.stringify(args)})\n`);
    try {
      const out = await rt.__smwExecuteTool(tool, ir.meta, args);
      const rendered = JSON.stringify(out, null, 2);
      process.stdout.write(
        rendered.length > 1600
          ? rendered.slice(0, 1600) + "\n  ...[truncated]\n"
          : rendered + "\n",
      );
      const check = CHECKS[tool.name];
      const problem = check ? check(out) : null;
      if (problem) {
        failed++;
        process.stdout.write(`✗ ${problem}\n`);
      } else {
        process.stdout.write("✓ ok\n");
      }
    } catch (e) {
      failed++;
      process.stdout.write(`✗ threw: ${e.message}\n`);
    }
  }
  process.stdout.write(
    failed === 0
      ? "\nLIVE VERIFY PASS\n"
      : `\nLIVE VERIFY: ${failed} tool(s) failed\n`,
  );
  process.exit(failed === 0 ? 0 : 1);
}

const CHECKS = {
  list_pull_requests: (o) =>
    o.status === 200 &&
    Array.isArray(o.pull_requests) &&
    o.pull_requests.every((r) => r.title && r.url)
      ? null
      : "expected 200 + rows with title+url",
  list_issues: (o) =>
    o.status === 200 &&
    Array.isArray(o.issues) &&
    o.issues.every((r) => r.title && r.url)
      ? null
      : "expected 200 + rows with title+url",
  list_releases: (o) =>
    o.status === 200 &&
    Array.isArray(o.release_links) &&
    o.release_links.length > 0 &&
    o.release_links.every((r) => r.url)
      ? null
      : "expected 200 + at least one tag link with url",
  get_repo_files: (o) =>
    o.status === 200 && typeof o.files === "string" && o.files.length > 50
      ? null
      : "expected 200 + a file table read",
  get_pr_diffstat: (o) =>
    o.status === 200 &&
    o.lines_added &&
    o.lines_deleted != null &&
    o.lines_changed
      ? null
      : "expected 200 + lines_added/deleted/changed",
};

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
