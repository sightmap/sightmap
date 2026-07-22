#!/usr/bin/env node
"use strict";
const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const os = require("node:os");
const { execFileSync } = require("node:child_process");

if (process.env.SIGHTMAP_SKIP_DOWNLOAD === "1") {
  console.log("sightmap: skipping binary download (SIGHTMAP_SKIP_DOWNLOAD=1)");
  process.exit(0);
}

const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"));
const version = pkg.version.replace(/^v/, "");
const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
const archMap = { x64: "amd64", arm64: "arm64" };
const goPlatform = platformMap[process.platform];
const goArch = archMap[process.arch];

if (!goPlatform || !goArch) {
  console.error(
    `sightmap: unsupported platform ${process.platform}/${process.arch}. ` +
      `Install manually: https://github.com/sightmap/sightmap/releases`
  );
  process.exit(1);
}

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const archiveName = `sightmap_${goPlatform}_${goArch}${ext}`;
const releaseTag = `v${version}`;
const downloadURL = `https://github.com/sightmap/sightmap/releases/download/${releaseTag}/${archiveName}`;
const vendorDir = path.join(__dirname, "vendor");
const binaryName = isWindows ? "sightmap.exe" : "sightmap";
const binaryPath = path.join(vendorDir, binaryName);

if (fs.existsSync(binaryPath)) {
  try {
    fs.accessSync(binaryPath, fs.constants.X_OK);
    process.exit(0);
  } catch (_) {}
}
fs.mkdirSync(vendorDir, { recursive: true });
console.log(`sightmap: downloading ${downloadURL}`);

function download(url, dest, cb) {
  const file = fs.createWriteStream(dest);
  https
    .get(url, (res) => {
      if (res.statusCode === 301 || res.statusCode === 302) {
        file.close();
        fs.unlinkSync(dest);
        return download(res.headers.location, dest, cb);
      }
      if (res.statusCode !== 200) {
        file.close();
        return cb(new Error(`HTTP ${res.statusCode}`));
      }
      res.pipe(file);
      file.on("finish", () => file.close(cb));
    })
    .on("error", (err) => {
      try {
        fs.unlinkSync(dest);
      } catch (_) {}
      cb(err);
    });
}

const tmp = path.join(os.tmpdir(), archiveName);
download(downloadURL, tmp, (err) => {
  if (err) {
    console.error("sightmap: download failed:", err.message);
    process.exit(1);
  }
  try {
    if (isWindows) {
      execFileSync("powershell", [
        "-NoProfile",
        "-Command",
        `Expand-Archive -Force '${tmp}' '${vendorDir}'`,
      ]);
    } else {
      execFileSync("tar", ["-xzf", tmp, "-C", vendorDir, binaryName]);
    }
    fs.unlinkSync(tmp);
    if (!isWindows) fs.chmodSync(binaryPath, 0o755);
    console.log(`sightmap: installed ${binaryPath}`);
  } catch (e) {
    console.error("sightmap: extraction failed:", e.message);
    process.exit(1);
  }
});
