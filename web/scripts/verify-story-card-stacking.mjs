// Proves the story card wins the paint order against 3D labels.
//
//   pnpm stacking:verify                       # against a fresh dev server
//   pnpm stacking:verify -- --url=http://localhost:5173/building
//   pnpm stacking:verify -- --out=/tmp/stacking   # keep screenshots + JSON report
//
// The bug this guards against: .bld-overlay (labels, portalled out of the 3D
// stage) painting on top of .bld-story (the chapter's headline and body copy).
// At 390px the card covers most of the viewport, so a label beating it reads
// as the chapter itself being unreadable — reported against the #367 preview
// on an iPhone, chapters 03 (wayfinding floor tags) and 07 (web-mcp / sightkick
// tool chips).
//
// This is the first *committed* check on this stacking order. PR #369's
// acceptance evidence measured "label paints above card" as the improved
// number (0/27 -> 24/27) — correct for the spec it was built against, wrong
// the moment the card became the thing that has to win — but that measurement
// lived only in the PR description, never in the repo, so there is nothing in
// occlusion.test.ts or elsewhere to invert. This script is the assertion, and
// it asserts the opposite: .bld-story must out-rank .bld-overlay at every
// probed point.
//
// A label and a card essentially never share a pixel on their own — labels
// hang off 3D anchors, cards sit in a fixed reading column — so the probe does
// what PR #369's own evidence did: temporarily reposition a real, mounted
// `.bld-card` (via `position: fixed`, restored immediately after) on top of a
// real, visible label's screen position, then reads `elementsFromPoint` at
// that shared point. The card never leaves the DOM and never leaves
// `.bld-story`'s stacking context, so what is actually being compared is
// still `.bld-story` against `.bld-overlay` — exactly the two contexts the
// fix touches.
//
// Three traps this script exists to avoid, all hit previously on this release:
//
//   1. --disable-gpu kills the WebGL context outright; run SwiftShader instead
//      (see capture-posters.mjs) and never pass --disable-gpu.
//   2. elementsFromPoint silently skips pointer-events:none nodes, and every
//      label carries one. The probe re-enables hit-testing on .bld-overlay for
//      the read and restores its original inline style immediately after.
//   3. The scene's drift and damping are driven by performance.now(), and
//      demand-mode rendering (now on under prefers-reduced-motion) can freeze
//      it part-way. Capture mode is forced and reduced-motion is explicitly
//      set to "no-preference" so the scene actually settles before a probe.
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import puppeteer from "puppeteer-core";

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, "..");
const DEV_PORT = 5200;
const CHAPTERS_EXPECTED = 9;
const SETTLED_DELTA = 0.002;
const SETTLE_TIMEOUT_MS = 20_000;
const DWELL_MS = 900;

/** Matches the viewports the defect was triaged at. 390x844 is primary: the
 *  reported break was an iPhone at 390px. */
const VIEWPORTS = {
  desktop: { width: 1440, height: 900, scale: 1 },
  laptop: { width: 1024, height: 768, scale: 1 },
  mobile: { width: 390, height: 844, scale: 2 },
};

const CHROME_CANDIDATES = [
  process.env.CHROME,
  "/usr/bin/chromium",
  "/usr/bin/chromium-browser",
  "/usr/bin/google-chrome-stable",
  "/usr/bin/google-chrome",
].filter(Boolean);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function findChrome() {
  const found = CHROME_CANDIDATES.find((p) => existsSync(p));
  if (!found) {
    console.error(
      `no Chrome found. Tried:\n  ${CHROME_CANDIDATES.join("\n  ")}\nSet CHROME=/path/to/chrome.`,
    );
    process.exit(1);
  }
  return found;
}

function parseArgs(argv) {
  const args = { url: null, out: null };
  for (const arg of argv) {
    if (arg === "--") continue;
    const [key, value] = arg.replace(/^--/, "").split("=");
    if (key === "url") args.url = value;
    else if (key === "out") args.out = value;
    else {
      console.error(`unknown argument: ${arg}`);
      process.exit(1);
    }
  }
  return args;
}

async function waitForServer(url, timeoutMs = 120_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { redirect: "manual" });
      if (res.status < 500) return;
    } catch {
      // not listening yet
    }
    await sleep(400);
  }
  throw new Error(`dev server did not answer at ${url} within ${timeoutMs}ms`);
}

async function startDevServer() {
  const child = spawn(
    "pnpm",
    ["dev", "--port", String(DEV_PORT), "--strictPort"],
    {
      cwd: webRoot,
      stdio: ["ignore", "ignore", "inherit"],
    },
  );
  const url = `http://localhost:${DEV_PORT}/building`;
  await waitForServer(url);
  return { url, stop: () => child.kill("SIGTERM") };
}

async function loadHydrated(page, url) {
  for (let attempt = 1; attempt <= 4; attempt++) {
    await page.goto(url, { waitUntil: "networkidle2", timeout: 60_000 });
    try {
      await page.waitForFunction(
        (expected) =>
          document.querySelectorAll(".bld-chapter").length === expected &&
          !!document.querySelector(".bld-stage canvas"),
        { timeout: 20_000 },
        CHAPTERS_EXPECTED,
      );
      await page.waitForFunction(
        () => (window.__bldCaptureProgress?.frames ?? 0) > 0,
        { timeout: 30_000 },
      );
      return;
    } catch {
      console.warn(`  hydration incomplete (attempt ${attempt}) — reloading`);
    }
  }
  throw new Error(
    "page never hydrated with nine chapters and a drawing canvas",
  );
}

async function goToChapter(page, index, id) {
  await page.evaluate((i) => {
    const el = document.querySelectorAll(".bld-chapter")[i];
    const top = el.offsetTop + el.offsetHeight / 2 - window.innerHeight / 2;
    window.scrollTo({ top: Math.max(0, top), behavior: "instant" });
  }, index);
  await page.waitForFunction(
    (chapter) =>
      document.querySelector('.bld-chapter[data-active="true"]')?.dataset
        .chapter === chapter,
    { timeout: 10_000 },
    id,
  );
}

async function waitSettled(page) {
  await page.waitForFunction(
    (limit) => {
      const p = window.__bldCaptureProgress;
      return !!p && p.frames > 30 && p.delta < limit;
    },
    { timeout: SETTLE_TIMEOUT_MS, polling: 100 },
    SETTLED_DELTA,
  );
  await sleep(DWELL_MS);
}

/**
 * Forces a real card over a real label's screen position and reads the paint
 * order at that shared point. Restores the card's style and the overlay's
 * pointer-events before returning, whatever the outcome.
 */
async function probeStacking(page) {
  return page.evaluate(() => {
    const card = document.querySelector(
      '.bld-chapter[data-active="true"] .bld-card',
    );
    if (!card) return { skipped: true, reason: "no active card" };

    const overlay = document.querySelector(".bld-overlay");
    const labels = [...overlay.querySelectorAll(".bld-tag")].filter((el) => {
      const style = getComputedStyle(el);
      const rect = el.getBoundingClientRect();
      return style.display !== "none" && rect.width > 0 && rect.height > 0;
    });
    if (labels.length === 0)
      return {
        skipped: true,
        reason: "no visible label in this chapter/viewport",
      };

    const label = labels[0];
    const labelRect = label.getBoundingClientRect();
    const x = labelRect.left + labelRect.width / 2;
    const y = labelRect.top + labelRect.height / 2;
    const cardRect = card.getBoundingClientRect();

    const prevCardStyle = card.getAttribute("style");
    const prevOverlayPE = overlay.style.pointerEvents;
    try {
      // Reposition only — the card stays exactly where it is in the DOM, so
      // the stacking-context comparison underneath (.bld-story vs
      // .bld-overlay) is untouched by this probe.
      card.style.position = "fixed";
      card.style.left = `${x - cardRect.width / 2}px`;
      card.style.top = `${y - cardRect.height / 2}px`;
      card.style.right = "auto";
      card.style.bottom = "auto";
      card.style.margin = "0";
      // elementsFromPoint skips pointer-events:none — every label inherits it
      // from .bld-overlay. Paint order does not depend on hit-testing.
      overlay.style.pointerEvents = "auto";

      const stack = document.elementsFromPoint(x, y);
      const storyIdx = stack.findIndex((el) => el.closest(".bld-story"));
      const overlayIdx = stack.findIndex((el) => el.closest(".bld-overlay"));
      return {
        skipped: false,
        x,
        y,
        labelText: label.textContent?.trim().slice(0, 40),
        storyIdx,
        overlayIdx,
        pass: storyIdx !== -1 && overlayIdx !== -1 && storyIdx < overlayIdx,
        stackPreview: stack.slice(0, 6).map((el) => el.className || el.tagName),
      };
    } finally {
      if (prevCardStyle === null) card.removeAttribute("style");
      else card.setAttribute("style", prevCardStyle);
      overlay.style.pointerEvents = prevOverlayPE;
    }
  });
}

async function verifyViewport(browser, url, variant, outDir) {
  const { width, height, scale } = VIEWPORTS[variant];
  const page = await browser.newPage();
  await page.setViewport({ width, height, deviceScaleFactor: scale });
  await page.emulateMediaFeatures([
    { name: "prefers-reduced-motion", value: "no-preference" },
  ]);
  await page.evaluateOnNewDocument(() => {
    window.__bldCapture = true;
  });
  page.on("pageerror", (err) => console.warn(`  [page error] ${err.message}`));

  await loadHydrated(page, url);

  const chapters = await page.evaluate(() =>
    [...document.querySelectorAll(".bld-chapter")].map(
      (el) => el.dataset.chapter,
    ),
  );

  const results = [];
  for (const [index, id] of chapters.entries()) {
    await goToChapter(page, index, id);
    await waitSettled(page);

    if (outDir) {
      const frame = await page.screenshot({ type: "png" });
      writeFileSync(
        resolve(
          outDir,
          `${variant}-${String(index).padStart(2, "0")}-${id}.png`,
        ),
        frame,
      );
    }

    const probe = await probeStacking(page);
    results.push({ variant, index, id, ...probe });
    if (probe.skipped) {
      console.log(
        `  --  ${variant.padEnd(8)} ${String(index).padStart(2, "0")} ${id.padEnd(14)} skipped (${probe.reason})`,
      );
    } else {
      console.log(
        `  ${probe.pass ? "ok " : "FAIL"} ${variant.padEnd(8)} ${String(index).padStart(2, "0")} ${id.padEnd(14)} ` +
          `story@${probe.storyIdx} overlay@${probe.overlayIdx} "${probe.labelText}"`,
      );
    }
  }
  await page.close();
  return results;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const outDir = args.out ? resolve(args.out) : null;
  if (outDir) mkdirSync(outDir, { recursive: true });

  const chrome = findChrome();

  let server = null;
  let url = args.url;
  if (!url) {
    server = await startDevServer();
    url = server.url;
  } else {
    if (new URL(url).pathname === "/") url = new URL("/building", url).href;
    await waitForServer(url);
  }

  const browser = await puppeteer.launch({
    executablePath: chrome,
    headless: true,
    args: [
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--hide-scrollbars",
      // SwiftShader, NOT --disable-gpu — see capture-posters.mjs for why.
      "--use-gl=angle",
      "--use-angle=swiftshader",
      "--enable-unsafe-swiftshader",
    ],
  });

  try {
    const all = [];
    for (const variant of Object.keys(VIEWPORTS)) {
      const { width, height } = VIEWPORTS[variant];
      console.log(`\n${variant} (${width}x${height})`);
      all.push(...(await verifyViewport(browser, url, variant, outDir)));
    }

    const probed = all.filter((r) => !r.skipped);
    const passed = probed.filter((r) => r.pass);
    const failed = probed.filter((r) => !r.pass);

    if (outDir)
      writeFileSync(
        resolve(outDir, "report.json"),
        JSON.stringify(all, null, 2),
      );

    console.log(
      `\n${passed.length}/${probed.length} probes: story card paints above the label layer`,
    );
    if (all.length - probed.length > 0)
      console.log(
        `${all.length - probed.length} chapter/viewport pairs had no visible label to probe`,
      );

    if (failed.length > 0) {
      console.error("\nFailing probes:");
      for (const f of failed)
        console.error(
          `  ${f.variant} ch${f.index} (${f.id}): story@${f.storyIdx} overlay@${f.overlayIdx}`,
        );
      throw new Error(
        `${failed.length} probe(s) still show a label painting above the story card`,
      );
    }
  } finally {
    await browser.close();
    server?.stop();
  }
}

main().catch((err) => {
  console.error(`\nstacking verification failed: ${err.message}`);
  process.exit(1);
});
