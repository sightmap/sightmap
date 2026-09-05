// <SightkickTools/> — boot the Sightkick runtime on every page so the compiled
// tool layer registers on the browser's WebMCP surface (document.modelContext).
//
// Drop it in the root layout. Any WebMCP client then sees the app's tools:
// agent-browser (`agent-browser webmcp list`), the ChatGPT desktop browser,
// Chrome's own WebMCP surface. Nothing else in the app changes — the tools are
// compiled from `.sightmap/` + `.sightkick/` by `sightmap-next build`.
//
// Pass `ir` as the JSON object (imported at build time from
// public/.well-known/sightkick.json) to inline it and register synchronously
// on runtime load, or as a URL string to fetch it after hydration.
import React from "react";
import Script from "next/script";

const DEFAULT_IR = "/.well-known/sightkick.json";
const DEFAULT_RUNTIME = "/sightkick-runtime.js";

function bootScript(ir, runtime) {
  const irExpr = typeof ir === "string" ? null : JSON.stringify(ir);
  const irUrl = typeof ir === "string" ? JSON.stringify(ir) : null;
  return (
    "(function(){" +
    (irExpr ? `window.__sightkick_ir=${irExpr};` : "") +
    `var RT=${JSON.stringify(runtime)};` +
    `function load(){` +
    (irExpr
      ? `window.__sightkick.load(window.__sightkick_ir);`
      : `fetch(${irUrl},{cache:"no-store"}).then(function(r){return r.json()}).then(function(ir){window.__sightkick.load(ir)}).catch(function(e){console.warn("[sightkick] could not load IR",e)});`) +
    `}` +
    // An init script (agent-browser --init-script) may already have booted the runtime.
    `if(window.__sightkick){if(!window.__sightkick.ir)load();return}` +
    `var s=document.createElement("script");s.src=RT;s.async=true;` +
    (irExpr ? "" : `s.onload=load;`) +
    `document.head.appendChild(s);` +
    "})();"
  );
}

export function SightkickTools({
  ir = DEFAULT_IR,
  runtime = DEFAULT_RUNTIME,
  enabled = true,
  strategy = "afterInteractive",
} = {}) {
  if (!enabled) return null;
  return React.createElement(Script, {
    id: "sightkick-boot",
    strategy,
    dangerouslySetInnerHTML: { __html: bootScript(ir, runtime) },
  });
}

export default SightkickTools;
