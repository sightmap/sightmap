"use strict";
// Resolve the sightmap native binary, downloading it on first use.
//
// npm >= 11 blocks install lifecycle scripts (postinstall) by default, so the
// binary can no longer be fetched at install time. Instead the bin launcher
// calls ensureBinary() on first run: it returns a cached binary if present,
// otherwise downloads the matching goreleaser asset from the GitHub release for
// this package's version, extracts it, and atomically publishes it into a
// per-user cache. Subsequent runs hit the cache and are instant.

const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const https = require("node:https");
const { execFileSync } = require("node:child_process");

const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
const archMap = { x64: "amd64", arm64: "arm64" };

// cacheRoot picks a writable, per-user cache directory. A global (or sudo)
// install location often is not writable at run time, so we never cache inside
// the package directory.
function cacheRoot() {
  if (process.env.SIGHTMAP_CACHE_DIR) return process.env.SIGHTMAP_CACHE_DIR;
  if (process.platform === "win32") {
    const base = process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local");
    return path.join(base, "sightmap");
  }
  const base = process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache");
  return path.join(base, "sightmap");
}

function target() {
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
  const version = pkg.version.replace(/^v/, "");
  const goPlatform = platformMap[process.platform];
  const goArch = archMap[process.arch];
  if (!goPlatform || !goArch) {
    throw new Error(
      `unsupported platform ${process.platform}/${process.arch}. ` +
        "Download a binary manually: https://github.com/sightmap/sightmap/releases"
    );
  }
  const isWindows = process.platform === "win32";
  const binaryName = isWindows ? "sightmap.exe" : "sightmap";
  const dir = path.join(cacheRoot(), version);
  return { version, goPlatform, goArch, isWindows, binaryName, dir, binaryPath: path.join(dir, binaryName) };
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https
      .get(url, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) {
          file.close();
          try {
            fs.unlinkSync(dest);
          } catch (_) {}
          return download(res.headers.location, dest).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          file.close();
          try {
            fs.unlinkSync(dest);
          } catch (_) {}
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        res.pipe(file);
        file.on("finish", () => file.close(resolve));
      })
      .on("error", (err) => {
        try {
          fs.unlinkSync(dest);
        } catch (_) {}
        reject(err);
      });
  });
}

// ensureBinary returns the path to a runnable sightmap binary, fetching it once
// if needed. Set SIGHTMAP_BINARY to bypass the download and use a binary you
// supply yourself (CI, air-gapped installs).
async function ensureBinary() {
  if (process.env.SIGHTMAP_BINARY) return process.env.SIGHTMAP_BINARY;

  const t = target();
  try {
    fs.accessSync(t.binaryPath, fs.constants.X_OK);
    return t.binaryPath; // cached
  } catch (_) {}

  fs.mkdirSync(t.dir, { recursive: true });
  const ext = t.isWindows ? ".zip" : ".tar.gz";
  const archiveName = `sightmap_${t.goPlatform}_${t.goArch}${ext}`;
  const url = `https://github.com/sightmap/sightmap/releases/download/v${t.version}/${archiveName}`;
  const tmpArchive = path.join(t.dir, `.dl-${process.pid}-${Date.now()}${ext}`);
  const tmpExtract = fs.mkdtempSync(path.join(t.dir, ".x-"));

  process.stderr.write(`sightmap: fetching native binary (first run): ${archiveName}\n`);
  try {
    await download(url, tmpArchive);
    if (t.isWindows) {
      execFileSync("powershell", [
        "-NoProfile",
        "-Command",
        `Expand-Archive -Force '${tmpArchive}' '${tmpExtract}'`,
      ]);
    } else {
      execFileSync("tar", ["-xzf", tmpArchive, "-C", tmpExtract, t.binaryName]);
    }
    const extracted = path.join(tmpExtract, t.binaryName);
    if (!t.isWindows) fs.chmodSync(extracted, 0o755);
    // Publish atomically. If a concurrent first-run beat us to it, keep theirs.
    try {
      fs.renameSync(extracted, t.binaryPath);
    } catch (err) {
      if (!fs.existsSync(t.binaryPath)) throw err;
    }
  } finally {
    try {
      fs.rmSync(tmpExtract, { recursive: true, force: true });
    } catch (_) {}
    try {
      fs.unlinkSync(tmpArchive);
    } catch (_) {}
  }
  return t.binaryPath;
}

module.exports = { ensureBinary };
