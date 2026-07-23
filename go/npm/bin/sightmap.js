#!/usr/bin/env node
"use strict";
const { spawnSync } = require("node:child_process");
const { ensureBinary } = require("../resolve-binary");

(async () => {
  let binary;
  try {
    binary = await ensureBinary();
  } catch (err) {
    console.error("sightmap:", err.message);
    console.error(
      "sightmap: could not obtain the native binary. " +
        "Check your network, or download one from " +
        "https://github.com/sightmap/sightmap/releases and point SIGHTMAP_BINARY at it."
    );
    process.exit(1);
  }

  const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    if (result.error.code === "ENOENT") {
      console.error(`sightmap: binary not found at ${binary}`);
    } else {
      console.error("sightmap:", result.error.message);
    }
    process.exit(1);
  }
  process.exit(result.status ?? 1);
})();
