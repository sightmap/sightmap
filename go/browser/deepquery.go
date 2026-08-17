package browser

import _ "embed"

// DeepQueryJS defines shadow-piercing querySelector helpers, prepended to any
// browser-eval that must locate a node the way the offline matcher does: across
// shadow boundaries.
//
// Why this exists: capture is shadow-aware — probe.js walks node.shadowRoot and
// stamps data-sightmap-id on live shadow nodes, and the extracted tree flattens
// shadow subtrees in as ordinary children. So offline matching (and sel-probe's
// offline count, coverage, component queries) matches ACROSS shadow boundaries.
// A live document.querySelector does NOT cross shadow roots, so every live
// lookup that re-finds a matched node (property extraction, interaction,
// bounds, the sel-probe live count) was silently shadow-blind and disagreed
// with the corpus. These helpers close that gap. See schema.md "Selector model
// & shadow DOM".
//
// The traversal order matches probe.js's procNode exactly — a node's light-DOM
// children, fully (each recursively, including its own shadow content), before
// that node's own shadow-DOM children — so "first match" agrees with the
// corpus instead of surfacing shadow-DOM matches out of order.
//
//	__smDeepQueryAll(root, sel) — every element matching sel within root's tree
//	  AND every shadow tree nested beneath it. root may be the document, a
//	  shadow root, or an element; for an element the matches are its descendants
//	  (mirroring Element.querySelectorAll) plus descendants inside its own or any
//	  nested shadow root.
//	__smDeepQuery(root, sel) — the first such element, or null.
//
// Compose it once into any other embedded .js source that calls
// __smDeepQuery/__smDeepQueryAll (see e.g. actions.go's actionsJS), rather
// than prepending it again at each eval call site. The helpers are function
// declarations, so the script's trailing expression stays the completion
// value.
//
//go:embed deepquery.js
var DeepQueryJS string
