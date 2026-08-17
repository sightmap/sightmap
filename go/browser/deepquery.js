// Shadow-piercing querySelector helpers, prepended to any browser-eval that
// must locate a node the way the offline matcher does: across shadow
// boundaries, in the same order probe.js's procNode flattens them (a node's
// own light-DOM subtree, fully, before that node's shadow-DOM subtree). See
// deepquery.go for why this exists and schema.md's "Selector model & shadow
// DOM" for the flattening rule this mirrors.

function __smDeepQueryAll(root, sel) {
  const out = [];
  function visit(node) {
    for (const child of node.children) {
      if (child.matches(sel)) out.push(child);
      visit(child);
      if (child.shadowRoot) visit(child.shadowRoot);
    }
  }
  try {
    visit(root);
    if (root.shadowRoot) visit(root.shadowRoot);
  } catch (e) {
    return out;
  }
  return out;
}

function __smDeepQuery(root, sel) {
  const matches = __smDeepQueryAll(root, sel);
  return matches.length ? matches[0] : null;
}
