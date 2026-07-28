// Renders og/card.html to public/og-image.png at exactly 1200x630.
// Uses the system Chrome rather than Playwright so this adds no dependency.
import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

if (!existsSync(CHROME)) {
  console.error(`Chrome not found at ${CHROME}. Install Google Chrome to regenerate the card.`);
  process.exit(1);
}

const out = resolve(here, '../public/og-image.png');
execFileSync(CHROME, [
  '--headless', '--disable-gpu', '--hide-scrollbars',
  '--force-device-scale-factor=1',      // 1200x630 exactly, not a retina multiple
  '--window-size=1200,630',
  '--virtual-time-budget=10000',        // let the Google Fonts request settle
  '--allow-file-access-from-files',     // card.html loads lockup.svg from disk
  `--screenshot=${out}`,
  `file://${resolve(here, 'card.html')}`,
], { stdio: 'inherit' });

console.log(`wrote ${out}`);
