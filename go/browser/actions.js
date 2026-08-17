// Live-DOM action helpers used by actions.go. Depends on
// __smDeepQuery/__smDeepQueryAll from deepquery.js being prepended.

function __smBoundsBySelector(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return {x: Math.round(r.left), y: Math.round(r.top), width: Math.round(r.width), height: Math.round(r.height)};
}

// __smLocateForClick scrolls the element to the center of the viewport and
// reports whether its center is actually the top-most element there (so a
// target hidden behind an overlay is reported as not-hit rather than clicked
// through).
function __smLocateForClick(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el) return {found: false};
  el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
  const r = el.getBoundingClientRect();
  const cx = Math.floor(r.left + r.width / 2), cy = Math.floor(r.top + r.height / 2);
  const inViewport = r.width > 0 && r.height > 0 && cx >= 0 && cy >= 0 && cx < window.innerWidth && cy < window.innerHeight;
  let hit = false;
  if (inViewport) {
    const at = document.elementFromPoint(cx, cy);
    hit = !!at && (at === el || el.contains(at) || at.contains(el));
  }
  return {found: true, inViewport, hit, cx, cy};
}

function __smClickBySelector(sel) {
  __smDeepQuery(document, sel)?.click();
}

function __smReadValueBySelector(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el || el.value == null) return null;
  return String(el.value);
}

// __smScrollIntoViewBySelector is wrapped in try/catch so a syntactically
// invalid selector (e.g. a bare numeric probe ID) degrades silently rather
// than surfacing as an EvalJSON JS exception.
function __smScrollIntoViewBySelector(sel) {
  try {
    const el = __smDeepQuery(document, sel);
    if (el) el.scrollIntoView({block: 'center', behavior: 'instant'});
  } catch (_) {}
}

function __smSelectorExists(sel) {
  return !!__smDeepQuery(document, sel);
}

function __smLocationProtocol() {
  return location.protocol;
}

function __smScrollPosition() {
  return {x: window.scrollX, y: window.scrollY};
}

function __smScrollToY(y) {
  window.scrollTo(0, Math.max(0, y - Math.floor(window.innerHeight / 2)));
}

function __smScrollBy(deltaX, deltaY) {
  window.scrollBy(deltaX, deltaY);
}

// __smClearActiveElement clears document.activeElement's value via the native
// JS setter — works on React-controlled inputs where Ctrl+A may not select.
function __smClearActiveElement() {
  const el = document.activeElement;
  if (!el) return;
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value') ||
                 Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value');
  if (setter && setter.set) setter.set.call(el, '');
  el.dispatchEvent(new Event('input', {bubbles: true}));
  el.dispatchEvent(new Event('change', {bubbles: true}));
}
