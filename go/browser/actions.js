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

// __smFullyVisible reports whether el's box lies inside the viewport and inside
// the visible area of every ancestor that clips its overflow (a scrolling
// column, a modal body). The viewport alone is not enough: a card scrolled
// under its column's header is inside the viewport and still cannot be reached.
function __smFullyVisible(el) {
  const r = el.getBoundingClientRect();
  let left = 0,
    top = 0,
    right = window.innerWidth,
    bottom = window.innerHeight;
  const clips = /auto|scroll|hidden|clip|overlay/;
  for (let p = el.parentElement; p; p = p.parentElement) {
    const cs = getComputedStyle(p);
    const clipX = clips.test(cs.overflowX),
      clipY = clips.test(cs.overflowY);
    if (!clipX && !clipY) continue;
    const pr = p.getBoundingClientRect();
    if (clipX) {
      left = Math.max(left, pr.left);
      right = Math.min(right, pr.right);
    }
    if (clipY) {
      top = Math.max(top, pr.top);
      bottom = Math.min(bottom, pr.bottom);
    }
  }
  return (
    r.width > 0 &&
    r.height > 0 &&
    r.left >= left &&
    r.top >= top &&
    r.right <= right &&
    r.bottom <= bottom
  );
}

// __smDragPrepare picks the point a drag starts from, in viewport coordinates,
// and returns the element's bounds and the viewport size. The element is
// scrolled to the center of the viewport only when it is not already fully
// visible: scrolling a visible element moves the layout the caller measured the
// drop target in (an inner list scrolls under its header and the target's top
// ends up clipped).
//
// With an offset (offX/offY, from the element's top-left corner) that is the
// point; hit says whether the element is what is top-most there. Without one it
// is the first point of a coarse grid over the element whose top-most element
// is the element itself or a descendant that is not interactive (a chip, a
// button, a link, an input), because pressing those starts a click or a focus,
// not a drag. It falls back to the center. Returns null when the element is
// gone.
function __smDragPrepare(sel, offX, offY) {
  const el = __smDeepQuery(document, sel);
  if (!el) return null;
  if (!__smFullyVisible(el)) {
    el.scrollIntoView({
      block: "center",
      inline: "center",
      behavior: "instant",
    });
  }
  const r = el.getBoundingClientRect();
  const bounds = {
    x: Math.round(r.left),
    y: Math.round(r.top),
    width: Math.round(r.width),
    height: Math.round(r.height),
  };
  const view = { width: window.innerWidth, height: window.innerHeight };
  const onScreen = (x, y) =>
    x >= 0 && y >= 0 && x < view.width && y < view.height;
  const inside = (x, y) => {
    if (!onScreen(x, y)) return false;
    const at = document.elementFromPoint(x, y);
    return !!at && (at === el || el.contains(at));
  };
  if (offX != null && offY != null) {
    const x = bounds.x + offX,
      y = bounds.y + offY;
    return { bounds, view, x, y, hit: inside(x, y) };
  }
  const interactive =
    'button,a[href],input,select,textarea,label,summary,[role=button],[role=link],[role=checkbox],[role=menuitem],[role=switch],[contenteditable=""],[contenteditable=true]';
  const fracs = [0.5, 0.15, 0.85, 0.3, 0.7];
  for (const fy of fracs) {
    for (const fx of fracs) {
      const x = Math.floor(r.left + r.width * fx);
      const y = Math.floor(r.top + r.height * fy);
      if (!inside(x, y)) continue;
      const hot = document.elementFromPoint(x, y).closest(interactive);
      if (hot && hot !== el && el.contains(hot)) continue;
      return { bounds, view, x, y, hit: true };
    }
  }
  const x = Math.floor(r.left + r.width / 2),
    y = Math.floor(r.top + r.height / 2);
  return { bounds, view, x, y, hit: inside(x, y) };
}

// __smDropCheck says whether the viewport point (x, y) is on screen and is
// over el, the drop target (the element or one of its descendants is top-most
// there). A point inside the target's box can still be clipped by a scrolling
// ancestor or covered by a sticky header; elementFromPoint sees both. covered
// names what is there instead.
function __smDropCheck(sel, x, y) {
  const inViewport =
    x >= 0 && y >= 0 && x < window.innerWidth && y < window.innerHeight;
  if (!inViewport) return { inViewport: false, hit: false, covered: "" };
  const el = __smDeepQuery(document, sel);
  const at = document.elementFromPoint(x, y);
  const hit = !!el && !!at && (at === el || el.contains(at));
  let covered = "";
  if (!hit && at) {
    covered = at.tagName.toLowerCase();
    const comp = at.closest("[data-component]");
    if (comp)
      covered += "[data-component=" + comp.getAttribute("data-component") + "]";
    else if (at.id) covered += "#" + at.id;
  }
  return { inViewport: true, hit, covered };
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

// __smClearActiveElement clears document.activeElement's value via the native
// JS setter — works on React-controlled inputs where Ctrl+A may not select.
function __smClearActiveElement() {
  const el = document.activeElement;
  if (!el) return;
  const setter =
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value") ||
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value");
  if (setter && setter.set) setter.set.call(el, "");
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}
