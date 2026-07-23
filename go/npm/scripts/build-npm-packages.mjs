#!/usr/bin/env node
// Assemble the npm packages for a release from the goreleaser archives:
//   - one @sightmap/sightmap-<os>-<arch> package per platform (binary + os/cpu)
//   - the @sightmap/sightmap meta package (bin launcher + optionalDependencies)
// all pinned to the same version. Output is a staging directory whose
// subdirectories are each ready to `npm publish`.
//
// Usage:
//   node build-npm-packages.mjs --version 0.13.1 --dist <dir-of-archives> --out <staging>
//
// --version defaults to the meta package.json version (with any leading "v"
// stripped); --dist defaults to go/dist; --out defaults to <repo>/npm-staging.

import { execFileSync } from "node:child_process";
import {
  chmodSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const metaDir = resolve(here, ".."); // go/npm
const repoRoot = resolve(metaDir, "..", ".."); // repo root

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  return i !== -1 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
}

const commonMeta = JSON.parse(readFileSync(join(metaDir, "package.json"), "utf8"));
const version = (arg("version", commonMeta.version)).replace(/^v/, "");
const distDir = resolve(arg("dist", join(repoRoot, "go", "dist")));
const outDir = resolve(arg("out", join(repoRoot, "npm-staging")));

const SCOPE = "@sightmap";
// key uses Node's process.platform/process.arch values so the launcher can build
// the package name directly; archive uses goreleaser's os/arch naming.
const targets = [
  { key: "darwin-arm64", os: "darwin", cpu: "arm64", archive: "sightmap_darwin_arm64.tar.gz", bin: "sightmap" },
  { key: "darwin-x64", os: "darwin", cpu: "x64", archive: "sightmap_darwin_amd64.tar.gz", bin: "sightmap" },
  { key: "linux-arm64", os: "linux", cpu: "arm64", archive: "sightmap_linux_arm64.tar.gz", bin: "sightmap" },
  { key: "linux-x64", os: "linux", cpu: "x64", archive: "sightmap_linux_amd64.tar.gz", bin: "sightmap" },
  { key: "win32-arm64", os: "win32", cpu: "arm64", archive: "sightmap_windows_arm64.zip", bin: "sightmap.exe" },
  { key: "win32-x64", os: "win32", cpu: "x64", archive: "sightmap_windows_amd64.zip", bin: "sightmap.exe" },
];

function extractBinary(archive, destDir, binName) {
  const archivePath = join(distDir, archive);
  if (!existsSync(archivePath)) {
    throw new Error(`missing archive: ${archivePath}`);
  }
  mkdirSync(destDir, { recursive: true });
  if (archive.endsWith(".zip")) {
    // -j junk paths, -o overwrite: drop the single binary into destDir.
    execFileSync("unzip", ["-o", "-j", archivePath, binName, "-d", destDir], { stdio: "inherit" });
  } else {
    execFileSync("tar", ["-xzf", archivePath, "-C", destDir, binName]);
  }
  const binPath = join(destDir, binName);
  if (!existsSync(binPath)) {
    throw new Error(`archive ${archive} did not contain ${binName}`);
  }
  return binPath;
}

rmSync(outDir, { recursive: true, force: true });
mkdirSync(outDir, { recursive: true });

const optionalDependencies = {};
for (const t of targets) {
  const name = `${SCOPE}/sightmap-${t.key}`;
  const pkgDir = join(outDir, name);
  const binPath = extractBinary(t.archive, pkgDir, t.bin);
  if (t.os !== "win32") chmodSync(binPath, 0o755);

  const pkg = {
    name,
    version,
    description: `sightmap native binary for ${t.key}`,
    license: commonMeta.license,
    homepage: commonMeta.homepage,
    repository: commonMeta.repository,
    bugs: commonMeta.bugs,
    os: [t.os],
    cpu: [t.cpu],
    files: [t.bin],
    publishConfig: { access: "public" },
  };
  writeFileSync(join(pkgDir, "package.json"), JSON.stringify(pkg, null, 2) + "\n");
  optionalDependencies[name] = version;
  console.log(`staged ${name}@${version}`);
}

// Stage the meta package: copy launcher + README, pin version and the
// optionalDependencies to this release.
const metaOut = join(outDir, "meta");
mkdirSync(metaOut, { recursive: true });
cpSync(join(metaDir, "bin"), join(metaOut, "bin"), { recursive: true });
cpSync(join(metaDir, "README.md"), join(metaOut, "README.md"));

const meta = { ...commonMeta, version, optionalDependencies };
delete meta.scripts;
writeFileSync(join(metaOut, "package.json"), JSON.stringify(meta, null, 2) + "\n");
console.log(`staged ${meta.name}@${version} (meta)`);

console.log(`\nStaged ${targets.length} platform packages + meta under ${outDir}`);
