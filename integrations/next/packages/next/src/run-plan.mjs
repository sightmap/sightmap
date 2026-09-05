// run-plan — replay a stored plan through agent-browser, with no agent in the loop.
//
// A plan is a Gherkin scenario an agent resolved *once* against the tool layer:
// each line becomes a tool name, params, and an expectation. Replay is then a
// sequence of `agent-browser webmcp invoke` calls — no LLM, no tokens — until
// either the scenario text or the compiled IR changes, at which point the plan
// refuses to run and asks to be re-planned. The plan format and the expectation
// vocabulary are Sightkick's (examples/saucedemo/plans/*.plan.json in
// sightmap/sightkick); only the executor differs: agent-browser's native WebMCP
// path instead of `sightkick call`.
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "./args.mjs";
import { requireBin, run } from "./cli-tools.mjs";
import { WELL_KNOWN } from "./build.mjs";

const sha256 = (t) => "sha256:" + createHash("sha256").update(t).digest("hex");

/** Same vocabulary as sightkick's run-plan: ok, value.{equals,contains,absent}, list.{length,contains}. */
export function checkExpect(result, expect) {
  if (expect == null) return { pass: true };
  if ("ok" in expect && Boolean(result.ok) !== expect.ok) {
    return {
      pass: false,
      reason: `expected ok:${expect.ok}, got ok:${Boolean(result.ok)}`,
    };
  }
  if (expect.value) {
    const v = result.value;
    if (expect.value.absent && v !== undefined && v !== null && v !== "") {
      return {
        pass: false,
        reason: `expected value absent, got ${JSON.stringify(v)}`,
      };
    }
    if ("equals" in expect.value && v !== expect.value.equals) {
      return {
        pass: false,
        reason: `expected value ${JSON.stringify(expect.value.equals)}, got ${JSON.stringify(v)}`,
      };
    }
    if (
      expect.value.contains &&
      !(typeof v === "string" && v.includes(expect.value.contains))
    ) {
      return {
        pass: false,
        reason: `expected value to contain ${JSON.stringify(expect.value.contains)}, got ${JSON.stringify(v)}`,
      };
    }
  }
  if (expect.list) {
    const items = result.items ?? [];
    if ("length" in expect.list && items.length !== expect.list.length) {
      return {
        pass: false,
        reason: `expected ${expect.list.length} row(s), got ${items.length}`,
      };
    }
    if (expect.list.contains) {
      const found = items.some((row) =>
        Object.entries(expect.list.contains).every(([k, v]) => row[k] === v),
      );
      if (!found)
        return {
          pass: false,
          reason: `expected a row matching ${JSON.stringify(expect.list.contains)}, got ${JSON.stringify(items)}`,
        };
    }
    if (expect.list.excludes) {
      const found = items.some((row) =>
        Object.entries(expect.list.excludes).every(([k, v]) => row[k] === v),
      );
      if (found)
        return {
          pass: false,
          reason: `expected no row matching ${JSON.stringify(expect.list.excludes)}`,
        };
    }
  }
  return { pass: true };
}

/** Unwrap agent-browser's `webmcp invoke --json` envelope down to Sightkick's ToolResult. */
export function unwrapInvoke(stdout) {
  let env;
  try {
    env = JSON.parse(stdout);
  } catch {
    throw new Error(`agent-browser did not print JSON:\n${stdout}`);
  }
  if (env.success === false)
    return {
      ok: false,
      message: env.error?.message ?? JSON.stringify(env.error),
    };
  const out = env.data?.output ?? env.data;
  const text = out?.content?.find?.((c) => c.type === "text")?.text;
  if (typeof text !== "string") return { ok: !out?.isError, raw: out };
  try {
    return JSON.parse(text);
  } catch {
    return { ok: !out?.isError, value: text };
  }
}

class AgentBrowser {
  constructor(bin, { session, cwd }) {
    this.bin = bin;
    this.base = session ? ["--session", session] : [];
    this.cwd = cwd;
  }
  exec(args, opts = {}) {
    return run(this.bin, [...this.base, ...args], {
      cwd: this.cwd,
      quiet: true,
      ...opts,
    });
  }
  open(url, { initScript } = {}) {
    const args = ["open", url];
    if (initScript) args.unshift("--init-script", initScript);
    // The daemon may still be shutting down from a previous close; retry briefly.
    let last;
    for (let attempt = 0; attempt < 5; attempt++) {
      last = this.exec(args);
      if (last.status === 0) return;
      Atomics.wait(
        new Int32Array(new SharedArrayBuffer(4)),
        0,
        0,
        1000 * (attempt + 1),
      );
    }
    throw new Error(
      `agent-browser open ${url} failed:\n${last.stderr || last.stdout}`,
    );
  }
  waitForTools(timeoutMs = 15000) {
    // The app's <SightkickTools/> (or the init script) registers tools asynchronously.
    const r = this.exec([
      "wait",
      "--fn",
      "!!(window.__sightkick && window.__sightkick.ir)",
      "--timeout",
      String(timeoutMs),
    ]);
    if (r.status !== 0)
      throw new Error(
        `sightkick runtime never loaded an IR on the page (is <SightkickTools/> in the root layout, or --init-script set?)\n${r.stderr || r.stdout}`,
      );
  }
  invoke(tool, params) {
    const args = ["webmcp", "invoke", tool, "--json"];
    if (params && Object.keys(params).length)
      args.push("--params", JSON.stringify(params));
    const r = this.exec(args);
    return unwrapInvoke(r.stdout || r.stderr);
  }
  close() {
    this.exec(["close"]);
  }
}

export function runPlan(planPath, opts = {}) {
  const {
    baseUrl = process.env.SIGHTMAP_BASE_URL ?? "http://localhost:3000",
    stamp = false,
    staleOk = false,
    dryRun = false,
    initScript,
    session,
    keepOpen = false,
    log = console.log,
  } = opts;
  const plan = JSON.parse(readFileSync(planPath, "utf8"));
  const planDir = dirname(resolve(planPath));
  const root = resolve(planDir, plan.app ?? "..");

  const featureText = readFileSync(
    resolve(root, plan.scenario.feature),
    "utf8",
  );
  const scenarioHash = sha256(featureText);
  const irPath = resolve(root, opts.publicDir ?? "public", WELL_KNOWN.ir);
  let irHash = null;
  try {
    irHash = sha256(readFileSync(irPath, "utf8"));
  } catch {
    throw new Error(
      `no compiled IR at ${irPath} — run 'sightmap-next build' first`,
    );
  }

  if (stamp) {
    plan.scenario.hash = scenarioHash;
    plan.irHash = irHash;
    plan.generatedAt = new Date().toISOString();
    writeFileSync(planPath, JSON.stringify(plan, null, 2) + "\n");
    log(`stamped ${planPath}`);
    return 0;
  }
  if (plan.scenario.hash && plan.scenario.hash !== scenarioHash && !staleOk) {
    log(
      `✗ ${plan.scenario.feature} changed since this plan was stamped — re-plan it (or pass --stale-ok)`,
    );
    return 1;
  }
  if (plan.irHash && plan.irHash !== irHash && !staleOk) {
    log(
      `✗ the compiled tool layer (${WELL_KNOWN.ir}) changed since this plan was stamped — re-plan it (or pass --stale-ok)`,
    );
    return 1;
  }
  if (!plan.scenario.hash || !plan.irHash)
    log(
      `⚠ ${planPath} is unstamped — run with --stamp to lock its hashes. Proceeding anyway.`,
    );

  log(`\n${plan.scenario.name}`);
  const startUrl = baseUrl.replace(/\/$/, "") + (plan.start ?? "/");
  if (dryRun) {
    log(`  agent-browser open ${startUrl}`);
    for (const step of plan.steps) {
      if (!step.tool) {
        log(`  · ${step.gherkin}`);
        continue;
      }
      const p = step.params ? ` --params '${JSON.stringify(step.params)}'` : "";
      log(
        `  ${step.gherkin}\n    agent-browser webmcp invoke ${step.tool}${p}`,
      );
    }
    return 0;
  }

  const ab =
    opts.browser ??
    new AgentBrowser(
      requireBin(
        "agent-browser",
        root,
        "npm i -D agent-browser (then: agent-browser install)",
      ),
      { session, cwd: root },
    );
  // Every plan starts from a fresh document, so state resets even when the browser is shared.
  ab.open(startUrl, {
    initScript: initScript ? resolve(initScript) : undefined,
  });
  ab.waitForTools();
  let failed = false;
  try {
    for (const step of plan.steps) {
      if (!step.tool) {
        log(`  · ${step.gherkin}`);
        continue;
      }
      const result = ab.invoke(step.tool, step.params);
      const { pass, reason } = checkExpect(result, step.expect);
      log(`  ${pass ? "✓" : "✗"} ${step.gherkin}`);
      if (!pass) {
        log(`      ${reason}\n      result: ${JSON.stringify(result)}`);
        failed = true;
        break;
      }
    }
  } finally {
    if (!keepOpen && !opts.browser) ab.close();
  }
  return failed ? 1 : 0;
}

export function createBrowser(root, { session } = {}) {
  return new AgentBrowser(
    requireBin(
      "agent-browser",
      root,
      "npm i -D agent-browser (then: agent-browser install)",
    ),
    { session, cwd: root },
  );
}

export async function runPlanCommand(argv) {
  const args = parseArgs(argv, {
    booleans: ["stamp", "stale-ok", "dry-run", "keep-open"],
  });
  if (!args._.length)
    throw new Error(
      "usage: sightmap-next run-plan <plan.json ...> [--base-url URL] [--init-script FILE] [--session NAME] [--stamp] [--stale-ok] [--dry-run]",
    );
  let code = 0;
  const live = !args.stamp && !args["dry-run"];
  const browser = live
    ? createBrowser(process.cwd(), { session: args.session })
    : undefined;
  try {
    for (const p of args._) {
      const c = runPlan(p, {
        browser,
        baseUrl: args["base-url"],
        initScript: args["init-script"],
        session: args.session,
        stamp: !!args.stamp,
        staleOk: !!args["stale-ok"],
        dryRun: !!args["dry-run"],
        keepOpen: !!args["keep-open"],
      });
      if (c !== 0) code = c;
    }
  } finally {
    if (browser && !args["keep-open"]) browser.close();
  }
  return code;
}
