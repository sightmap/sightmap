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
// and returns the viewport size. The element is
// scrolled to the center of the viewport only when it is not already fully
// visible: scrolling a visible element moves the layout the caller measured the
// drop target in (an inner list scrolls under its header and the target's top
// ends up clipped). force scrolls it regardless: for an element in a frame,
// "fully visible" is judged in the frame's own viewport, and the frame itself
// can still be off the top page's screen.
//
// With an offset (offX/offY, from the element's top-left corner) that is the
// point; hit says whether the element is what is top-most there. Without one it
// is the first point of a coarse grid over the element whose top-most element
// is the element itself or a descendant that is not interactive (a chip, a
// button, a link, an input), because pressing those starts a click or a focus,
// not a drag. It falls back to the center. Returns null when the element is
// gone.
function __smDragPrepare(sel, offX, offY, force) {
  const el = __smDeepQuery(document, sel);
  if (!el) return null;
  if (force || !__smFullyVisible(el)) {
    el.scrollIntoView({
      block: "center",
      inline: "center",
      behavior: "instant",
    });
  }
  const r = el.getBoundingClientRect();
  const view = { width: window.innerWidth, height: window.innerHeight };
  const onScreen = (x, y) =>
    x >= 0 && y >= 0 && x < view.width && y < view.height;
  const inside = (x, y) => {
    if (!onScreen(x, y)) return false;
    const at = document.elementFromPoint(x, y);
    return !!at && (at === el || el.contains(at));
  };
  if (offX != null && offY != null) {
    const x = Math.round(r.left) + offX,
      y = Math.round(r.top) + offY;
    return { view, x, y, hit: inside(x, y) };
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
      return { view, x, y, hit: true };
    }
  }
  const x = Math.floor(r.left + r.width / 2),
    y = Math.floor(r.top + r.height / 2);
  return { view, x, y, hit: inside(x, y) };
}

// __smDropCheck says whether the viewport point (x, y) is on screen and is
// over el, the drop target (the element or one of its descendants is top-most
// there). A point inside the target's box can still be clipped by a scrolling
// ancestor or covered by a sticky header; elementFromPoint sees both. covered
// names what is there instead.
function __smDropCheck(sel, x, y) {
  const inViewport =
    x >= 0 && y >= 0 && x < window.innerWidth && y < window.innerHeight;
  if (!inViewport) return { hit: false, covered: "" };
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
  return { hit, covered };
}

// __smLocateForClick scrolls the element to the center of the viewport and
// reports whether its center is actually the top-most element there (so a
// target hidden behind an overlay is reported as not-hit rather than clicked
// through).
//
// A target already wholly visible (inside the viewport and every clipping
// ancestor, __smFullyVisible) is not scrolled at all, even when something covers
// it: scrolling can't uncover it, and the hit test below reports it as covered.
// One that is scrolled is brought into view horizontally only as far as needed
// ("nearest"), never centered:
// centering scrolls every scrollable ancestor, including an overflow:hidden
// one that is wider than its box only because a tooltip overflows it. That
// moves the content under a still mouse, which closes the tooltip, which
// snaps the scroll back, and the click lands where the target was a moment
// before (on its neighbour).
// force scrolls even a visible target: an element visible in its own frame can
// sit in a frame that is itself off the top page's screen, and only a scroll
// started from the element (which scrolls the enclosing documents too) brings
// it into view. The caller asks for it after measuring the frame's position.
function __smLocateForClick(sel, force) {
  const el = __smDeepQuery(document, sel);
  if (!el) return { found: false };
  const hits = (r) => {
    const at = document.elementFromPoint(
      Math.floor(r.left + r.width / 2),
      Math.floor(r.top + r.height / 2),
    );
    return !!at && (at === el || el.contains(at) || at.contains(el));
  };
  let r = el.getBoundingClientRect();
  if (force || !__smFullyVisible(el)) {
    el.scrollIntoView({
      block: "center",
      inline: "nearest",
      behavior: "instant",
    });
    r = el.getBoundingClientRect();
  }
  const cx = Math.floor(r.left + r.width / 2),
    cy = Math.floor(r.top + r.height / 2);
  const inViewport =
    r.width > 0 &&
    r.height > 0 &&
    cx >= 0 &&
    cy >= 0 &&
    cx < window.innerWidth &&
    cy < window.innerHeight;
  const hit = inViewport && hits(r);
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

// __smFocusBySelector gives the element keyboard focus (and reports whether it
// exists), for a keypress that should land on it without a click.
function __smFocusBySelector(sel) {
  const el = __smDeepQuery(document, sel);
  if (!el) return false;
  el.scrollIntoView({ block: "center", inline: "center", behavior: "instant" });
  if (typeof el.focus === "function") el.focus();
  return true;
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

// __smNativeSetValue sets el.value through the setter of the element's own
// prototype (input, textarea or select: HTMLInputElement's setter called on a
// textarea throws "Illegal invocation"), which bypasses the instance-level
// value tracking a framework like React installs (assigning el.value directly
// is swallowed there), then fires the input and change events a user's edit
// would.
function __smNativeSetValue(el, value) {
  const proto = [HTMLInputElement, HTMLTextAreaElement, HTMLSelectElement].find(
    (C) => el instanceof C,
  )?.prototype;
  const setter = proto && Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (!setter) {
    throw new Error(
      "the focused <" + el.tagName.toLowerCase() + "> has no value to set",
    );
  }
  setter.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

// __smIsNativeValueType says whether an <input> type is one whose entry
// keystrokes cannot produce: a date, time or month control edits through
// segmented fields that swallow typed characters (typing 2027-04-16 into a date
// holding 2027-04-09 leaves "270416-04-09"), and a color picker or slider has no
// text to type. `fill` sets their value instead of typing it. (A function, not
// a top-level const: this script is evaluated again on every call, in the same
// page, where a second const declaration is a SyntaxError.)
function __smIsNativeValueType(type) {
  return [
    "date",
    "time",
    "datetime-local",
    "month",
    "week",
    "color",
    "range",
  ].includes(type);
}

// __smSetValueIfNative sets the value of the <input> carrying the selector
// natively, when __smIsNativeValueType, and fires input and change.
// It returns {native:false} for anything else (text inputs keep the keystroke
// path). Otherwise it reports the input's type and the value it holds
// afterwards: the browser sanitizes a value it cannot parse (a malformed date)
// to "" and clamps one out of range, so the caller sees what actually took.
// `blocked` is set for a disabled or read-only control a user could not edit.
function __smSetValueIfNative(sel, value) {
  const el = __smDeepQuery(document, sel);
  if (!el || !(el instanceof HTMLInputElement)) return { native: false };
  const type = (el.type || "text").toLowerCase();
  if (!__smIsNativeValueType(type)) return { native: false };
  if (el.disabled || el.readOnly) return { native: true, type, blocked: true };
  __smNativeSetValue(el, value);
  return { native: true, type, blocked: false, value: el.value };
}

// __smFocusProblem says why the focused element cannot be typed into, or is
// null when focus is on a field. `nothing` means focus is on the page itself
// (activeElement is BODY): what a click leaves behind when it never reached the
// page (a hidden tab) or landed on something that cannot take focus.
function __smFocusProblem() {
  const a = __smActiveElement();
  if (!a || a === document.body || a === document.documentElement) {
    return {
      nothing: true,
      reason: `nothing has focus (activeElement is ${a ? a.tagName : "absent"}, visibilityState=${document.visibilityState})`,
    };
  }
  if (!(a instanceof HTMLInputElement || a instanceof HTMLTextAreaElement)) {
    return {
      nothing: false,
      reason: `focus is on <${a.tagName.toLowerCase()}>, not an input or textarea`,
    };
  }
  return null;
}

// __smClearActiveElement empties the focused input, textarea, select or
// contenteditable and returns {ok:true, old} with what it held, or
// {ok:false, reason} when nothing it can clear has focus (so the caller can say
// the field did not take focus, rather than the page's exception). Inputs go
// through __smNativeSetValue, which is what React-controlled fields listen to.
function __smClearActiveElement() {
  const el = __smActiveElement();
  const problem = __smFocusProblem();
  if (
    problem &&
    (problem.nothing ||
      !(el.isContentEditable || el instanceof HTMLSelectElement))
  ) {
    return { ok: false, reason: problem.reason };
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
    return { ok: true, old };
  }
  const old = String(el.value);
  __smNativeSetValue(el, "");
  return { ok: true, old };
}
