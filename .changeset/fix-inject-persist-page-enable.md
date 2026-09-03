---
"@sightmap/sightmap": patch
---

Fix `browser inject --persist` not re-injecting across navigations. The persisted script was registered with `Page.addScriptToEvaluateOnNewDocument` on the daemon's collector connection, but that connection never enabled the Page domain — so the command returned an identifier (and `inject --list` showed the entry) while the script was never actually evaluated on new documents. Only the initial one-shot run fired. The collector now enables Page before registering, so a persisted script runs at document start on every subsequent navigation and new tab, as documented.
