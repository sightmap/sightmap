// Registers the Sightmap DevTools panel (bootstrap).
chrome.devtools.panels.create(
  "Sightmap",
  "",          // icon (none yet)
  "panel.html",
  (panel) => {
    console.log("[sightmap] devtools panel registered", panel);
  }
);
