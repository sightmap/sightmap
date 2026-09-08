// Shadow-piercing querySelector helpers, prepended to any browser-eval that
// must locate a node the way the offline matcher does: across shadow
// boundaries, in the same order probe.js's procNode flattens them (a node's
// own light-DOM subtree, fully, before that node's shadow-DOM subtree). See
// deepquery.go for why this exists and schema.md's "Selector model & shadow
// DOM" for the flattening rule this mirrors.
//
// DOM access goes through cached prototype accessors, NOT instance properties
// (node.children / el.matches / el.shadowRoot). An <input name="children"> makes
// HTMLFormElement's named getter return that control for form.children instead
// of the HTMLCollection, so a naive `for (const c of node.children)` throws
// "children is not iterable" on that form and the blanket catch below then
// returns a TRUNCATED result — silently 0 for any match after the form. (JetBlue's
// booker form has a "children" passenger-count control; that one collision made
// sel-probe and click/fill under-report.) The cached prototype accessors bypass
// any such named-property shadowing entirely.
//
// The accessors are cached INSIDE the function (not at top level): this file is
// prepended to CDP evals that run repeatedly in one tab context, so it must
// declare no top-level lexical bindings (a second eval of a top-level `const`
// throws "already declared"). Function declarations are re-eval-safe; keep it
// that way. See deepquery.go.
function __smDeepQueryAll(root, sel) {
  const childNodesOf = Object.getOwnPropertyDescriptor(
    Node.prototype,
    "childNodes",
  ).get;
  const matchesFn = Element.prototype.matches;
  const shadowRootGetter = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "shadowRoot",
  ).get;
  // shadowRoot's getter lives on Element.prototype, so it is only valid for
  // element nodes (a Document/ShadowRoot root has none).
  const shadowOf = (node) =>
    node.nodeType === 1 ? shadowRootGetter.call(node) : null;

  const out = [];
  function visit(node) {
    for (const child of childNodesOf.call(node)) {
      if (child.nodeType !== 1) continue; // ELEMENT_NODE only
      if (matchesFn.call(child, sel)) out.push(child);
      visit(child);
      const sr = shadowOf(child);
      if (sr) visit(sr);
    }
  }
  try {
    visit(root);
    const rootShadow = shadowOf(root);
    if (rootShadow) visit(rootShadow);
  } catch (e) {
    return out;
  }
  return out;
}

function __smDeepQuery(root, sel) {
  const matches = __smDeepQueryAll(root, sel);
  return matches.length ? matches[0] : null;
}
