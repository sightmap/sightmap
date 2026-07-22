// Adapted from archcore-ai/docs.archcore.ai (Apache-2.0).
// Source: https://github.com/archcore-ai/docs.archcore.ai/blob/main/scripts/generate-og-image.mts
// Generates 1200x630 OG images for each docs page.
import satori from "satori";
import { Resvg } from "@resvg/resvg-js";
import { readFileSync, writeFileSync, mkdirSync, readdirSync, statSync, existsSync } from "fs";
import { join, dirname, relative } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const rootDir = join(__dirname, "..");
const publicDir = join(rootDir, "public");
const docsDir = join(rootDir, "src", "content", "docs");

const dmSansRegular = readFileSync(join(__dirname, "fonts", "DMSans-Regular.ttf"));
const dmSansBold = readFileSync(join(__dirname, "fonts", "DMSans-Bold.ttf"));
const jetBrainsMono = readFileSync(join(__dirname, "fonts", "JetBrainsMono-Bold.ttf"));

const WIDTH = 1200;
const HEIGHT = 630;

// Sightmap palette — warm cream + dark text + accent pink/red.
const BG = "#faf8f6";
const TEXT = "#1a1714";
const TEXT_DIM = "#8a8272";
const ACCENT = "#c9456d";
const BORDER = "#e5e0da";

const SECTION_MAP: Record<string, string> = {
  start: "Start Here",
  spec: "The Spec",
  authoring: "Authoring",
  concepts: "Concepts",
  reference: "Reference",
};

const fonts = [
  { name: "DM Sans", data: dmSansRegular, weight: 400 as const, style: "normal" as const },
  { name: "DM Sans", data: dmSansBold, weight: 700 as const, style: "normal" as const },
  { name: "JetBrains Mono", data: jetBrainsMono, weight: 700 as const, style: "normal" as const },
];

interface PageInfo {
  slug: string;
  title: string;
  description?: string;
  section?: string;
}

function parseFrontmatter(content: string): { title?: string; description?: string } {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!match) return {};
  const yaml = match[1];
  const title = yaml.match(/^title:\s*['"]?(.*?)['"]?\s*$/m)?.[1];
  const description = yaml.match(/^description:\s*['"]?(.*?)['"]?\s*$/m)?.[1];
  return { title, description };
}

function collectFiles(dir: string, exts: string[]): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...collectFiles(full, exts));
    else if (exts.some((e) => full.endsWith(e))) out.push(full);
  }
  return out;
}

function discoverPages(): PageInfo[] {
  const pages: PageInfo[] = [];
  for (const file of collectFiles(docsDir, [".md", ".mdx"])) {
    const rel = relative(docsDir, file);
    const slug = rel.replace(/\.(mdx?|md)$/, "").replace(/\/index$/, "");
    const finalSlug = slug === "index" ? "" : slug;
    const content = readFileSync(file, "utf-8");
    const { title, description } = parseFrontmatter(content);
    if (!title) continue;
    const section = finalSlug.includes("/") ? finalSlug.split("/")[0] : undefined;
    pages.push({
      slug: finalSlug,
      title,
      description,
      section: section ? SECTION_MAP[section] : undefined,
    });
  }
  return pages;
}

function createOgNode(title: string, description?: string, breadcrumb?: string) {
  // Satori accepts a JSX-like JSON tree. The brand mark is rendered as styled text
  // (no logo file needed).
  return {
    type: "div",
    props: {
      style: {
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        background: BG,
        padding: "60px 72px",
        fontFamily: "DM Sans",
      },
      children: [
        // Top: brand mark + section breadcrumb
        {
          type: "div",
          props: {
            style: { display: "flex", alignItems: "center", justifyContent: "space-between", color: TEXT },
            children: [
              {
                type: "div",
                props: {
                  style: { fontFamily: "JetBrains Mono", fontWeight: 700, fontSize: 28, letterSpacing: "-0.02em", display: "flex" },
                  children: [
                    { type: "span", props: { children: ".sightmap" } },
                    { type: "span", props: { style: { color: ACCENT }, children: "/" } },
                  ],
                },
              },
              breadcrumb
                ? {
                    type: "div",
                    props: {
                      style: { fontFamily: "JetBrains Mono", fontSize: 18, color: TEXT_DIM, textTransform: "uppercase", letterSpacing: "0.08em" },
                      children: breadcrumb,
                    },
                  }
                : { type: "div", props: { children: "" } },
            ],
          },
        },
        // Middle: title (top-aligned to give room for description)
        {
          type: "div",
          props: {
            style: { display: "flex", flex: "1", flexDirection: "column", justifyContent: "center", marginTop: 40 },
            children: [
              {
                type: "div",
                props: {
                  style: { fontFamily: "DM Sans", fontWeight: 700, fontSize: 72, lineHeight: 1.1, color: TEXT, letterSpacing: "-0.03em" },
                  children: title,
                },
              },
              description
                ? {
                    type: "div",
                    props: {
                      style: { fontFamily: "DM Sans", fontSize: 28, lineHeight: 1.45, color: TEXT_DIM, marginTop: 28 },
                      children: description,
                    },
                  }
                : { type: "div", props: { children: "" } },
            ],
          },
        },
        // Bottom: hairline + small URL
        {
          type: "div",
          props: {
            style: { borderTop: `1px solid ${BORDER}`, paddingTop: 20, fontFamily: "JetBrains Mono", fontSize: 16, color: TEXT_DIM },
            children: "docs.sightmap.org",
          },
        },
      ],
    },
  };
}

async function generateImage(
  title: string,
  outputPath: string,
  description?: string,
  breadcrumb?: string,
): Promise<void> {
  const node = createOgNode(title, description, breadcrumb);
  const svg = await satori(node as any, { width: WIDTH, height: HEIGHT, fonts });
  const resvg = new Resvg(svg, { fitTo: { mode: "width", value: WIDTH } });
  const png = resvg.render().asPng();
  mkdirSync(dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, png);
}

async function main() {
  const ogDir = join(publicDir, "og");
  mkdirSync(ogDir, { recursive: true });

  const pages = discoverPages();
  console.log(`Generating OG images for ${pages.length} pages...`);

  for (const page of pages) {
    const fileName = page.slug === "" ? "index.png" : `${page.slug.replace(/\//g, "-")}.png`;
    const outputPath = join(ogDir, fileName);
    await generateImage(page.title, outputPath, page.description, page.section);
    console.log(`  → ${fileName}`);
  }

  // Backward-compatible root image
  if (existsSync(join(ogDir, "index.png"))) {
    const rootImage = readFileSync(join(ogDir, "index.png"));
    writeFileSync(join(publicDir, "og-image.png"), rootImage);
  }

  console.log(`Done. Wrote ${pages.length} images.`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
