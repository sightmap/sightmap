// Live-DOM action helpers used by actions.go. Depends on
// __smDeepQuery/__smDeepQueryAll from deepquery.js being prepended.

function __smBoundsBySelector(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return {
    x: Math.round(r.left),
    y: Math.round(r.top),
    width: Math.round(r.width),
    height: Math.round(r.height),
  };
}

// __smLocateForClick scrolls the element to the center of the viewport and
// reports whether its center is actually the top-most element there (so a
// target hidden behind an overlay is reported as not-hit rather than clicked
// through).
function __smLocateForClick(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el) return { found: false };
  el.scrollIntoView({ block: "center", inline: "center", behavior: "instant" });
  const r = el.getBoundingClientRect();
  const cx = Math.floor(r.left + r.width / 2),
    cy = Math.floor(r.top + r.height / 2);
  const inViewport =
    r.width > 0 &&
    r.height > 0 &&
    cx >= 0 &&
    cy >= 0 &&
    cx < window.innerWidth &&
    cy < window.innerHeight;
  let hit = false;
  if (inViewport) {
    const at = document.elementFromPoint(cx, cy);
    hit = !!at && (at === el || el.contains(at) || at.contains(el));
  }
  return { found: true, inViewport, hit, cx, cy };
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
    if (el) el.scrollIntoView({ block: "center", behavior: "instant" });
  } catch (_) {}
}

function __smSelectorExists(sel) {
  return !!__smDeepQuery(document, sel);
}

function __smLocationProtocol() {
  return location.protocol;
}

function __smScrollPosition() {
  return { x: window.scrollX, y: window.scrollY };
}

function __smScrollToY(y) {
  window.scrollTo(0, Math.max(0, y - Math.floor(window.innerHeight / 2)));
}

function __smScrollBy(deltaX, deltaY) {
  window.scrollBy(deltaX, deltaY);
}

// __smActiveElement is the focused element, looking through open shadow roots.
function __smActiveElement() {
  let el = document.activeElement;
  while (el && el.shadowRoot && el.shadowRoot.activeElement) {
    el = el.shadowRoot.activeElement;
  }
  return el;
}

// __smSelectActive selects all the content of the focused input, textarea or
// contenteditable, so the next typed text replaces it. It returns whether it
// could, and the content as it was, for the caller to compare with afterwards.
// Doing it through the DOM works on every platform; a Ctrl+A key event does not
// select on macOS Chrome unless the editing command is sent with it.
function __smSelectActive() {
  const el = __smActiveElement();
  if (!el || el === document.body || el === document.documentElement) {
    return { selected: false, value: "" };
  }
  if (typeof el.select === "function" && el.value != null) {
    el.select();
    return { selected: true, value: String(el.value) };
  }
  if (el.isContentEditable) {
    const sel = el.ownerDocument.getSelection();
    const range = el.ownerDocument.createRange();
    range.selectNodeContents(el);
    sel.removeAllRanges();
    sel.addRange(range);
    return { selected: true, value: el.textContent || "" };
  }
  return { selected: false, value: "" };
}

// __smClearActiveElement empties the focused input, textarea, select or
// contenteditable and returns what it held, or throws when it cannot. Inputs go
// through the native value setter of the element's own prototype, which is what
// React-controlled fields listen to: HTMLInputElement's setter called on a
// textarea throws "Illegal invocation", so the setter is picked by the
// element, not by whichever prototype has one.
function __smClearActiveElement() {
  const el = __smActiveElement();
  if (!el || el === document.body || el === document.documentElement) {
    throw new Error("nothing is focused to clear");
  }
  if (el.isContentEditable && el.value == null) {
    const old = el.textContent || "";
    const sel = el.ownerDocument.getSelection();
    const range = el.ownerDocument.createRange();
    range.selectNodeContents(el);
    sel.removeAllRanges();
    sel.addRange(range);
    range.deleteContents();
    el.dispatchEvent(
      new InputEvent("input", { bubbles: true, inputType: "deleteContent" }),
    );
    return old;
  }
  const proto = [HTMLInputElement, HTMLTextAreaElement, HTMLSelectElement].find(
    (C) => el instanceof C,
  )?.prototype;
  const setter = proto && Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (!setter) {
    throw new Error(
      "the focused <" + el.tagName.toLowerCase() + "> has no value to clear",
    );
  }
  const old = String(el.value);
  setter.call(el, "");
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return old;
}
