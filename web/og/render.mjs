// Renders og/card.html (the site card) and og/post-card.html (per-post blog
// cards) to PNG at exactly 1200x630, using the system Chrome rather than
// Playwright/Puppeteer so this adds no dependency. The font-loading guard
// inlined in both HTML files (see font-guard.js) is what makes that safe:
// a blocked or slow Google Fonts fetch stamps a red banner into the output
// instead of silently shipping a wrong-typeface PNG.
//
//   node og/render.mjs                # site card -> public/og-image.png
//   node og/render.mjs --post <slug>  # blog card -> public/blog/og/<slug>.png
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import matter from 'gray-matter';

const here = dirname(fileURLToPath(import.meta.url));

// Common install locations across macOS and Linux, plus an explicit escape
// hatch via the CHROME env var for anything nonstandard (custom install dir,
// a wrapper script, etc).
const CHROME_CANDIDATES = [
  process.env.CHROME,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium-browser',
  '/usr/bin/chromium',
  '/snap/bin/chromium',
].filter(Boolean);

function findChrome() {
  return CHROME_CANDIDATES.find((path) => existsSync(path)) ?? null;
}

function formatDate(date) {
  return new Date(`${date}T00:00:00`).toLocaleDateString('en-US', {
    month: 'long',
    year: 'numeric',
  });
}

function screenshot(chrome, url, out) {
  execFileSync(chrome, [
    '--headless', '--disable-gpu', '--hide-scrollbars',
    '--force-device-scale-factor=1',      // 1200x630 exactly, not a retina multiple
    '--window-size=1200,630',
    '--virtual-time-budget=10000',        // gives the font-loading guard time to settle (success or failure) before the screenshot
    '--allow-file-access-from-files',     // cards load lockup.svg (and post-card.html loads font-guard.js) from disk
    `--screenshot=${out}`,
    url,
  ], { stdio: 'inherit' });
}

function renderSiteCard(chrome) {
  const out = resolve(here, '../public/og-image.png');
  screenshot(chrome, `file://${resolve(here, 'card.html')}`, out);
  console.log(`wrote ${out}`);
}

function renderPostCard(chrome, slug) {
  const postFile = resolve(here, '../content/blog', `${slug}.md`);
  if (!existsSync(postFile)) {
    console.error(`no post found at ${postFile}`);
    process.exit(1);
  }

  const { data } = matter(readFileSync(postFile, 'utf-8'));
  const { title, topic, author, date } = data;
  for (const [key, value] of Object.entries({ title, topic, author, date })) {
    if (!value) {
      console.error(`post "${slug}" is missing frontmatter field "${key}"`);
      process.exit(1);
    }
  }

  // Query params on a file:// URL: Chrome's URL parser exposes them via
  // location.search even though nothing resolves them against the
  // filesystem, so post-card.html's inline script can read them directly.
  const params = new URLSearchParams({ title, topic, author, date: formatDate(date) });

  const outDir = resolve(here, '../public/blog/og');
  mkdirSync(outDir, { recursive: true });
  const out = resolve(outDir, `${slug}.png`);
  screenshot(chrome, `file://${resolve(here, 'post-card.html')}?${params.toString()}`, out);
  console.log(`wrote ${out}`);
}

function main() {
  const chrome = findChrome();
  if (!chrome) {
    console.error(
      `Chrome not found. Checked: ${CHROME_CANDIDATES.join(', ')}. Install Google Chrome or Chromium, or set the CHROME env var to its binary path.`
    );
    process.exit(1);
  }

  const args = process.argv.slice(2);
  const postFlagIndex = args.indexOf('--post');
  if (postFlagIndex === -1) {
    renderSiteCard(chrome);
    return;
  }

  const slug = args[postFlagIndex + 1];
  if (!slug) {
    console.error('usage: node og/render.mjs --post <slug>');
    process.exit(1);
  }
  renderPostCard(chrome, slug);
}

main();
