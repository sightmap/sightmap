const fs = require("fs");
const path = require("path");

(0, eval)(fs.readFileSync(path.join(__dirname, "deepquery.js"), "utf8"));
(0, eval)(fs.readFileSync(path.join(__dirname, "actions.js"), "utf8"));

describe("__smBoundsBySelector", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("returns rounded bounds for the matched element", () => {
    document.body.innerHTML = '<div id="el"></div>';
    const el = document.getElementById("el");
    el.getBoundingClientRect = () => ({
      left: 1.4,
      top: 2.6,
      width: 10.5,
      height: 20.5,
    });

    expect(__smBoundsBySelector("#el")).toEqual({
      x: 1,
      y: 3,
      width: 11,
      height: 21,
    });
  });

  test("returns null when nothing matches", () => {
    document.body.innerHTML = "<div></div>";
    expect(__smBoundsBySelector("#missing")).toBeNull();
  });
});

describe("__smLocateForClick", () => {
  afterEach(() => {
    document.body.innerHTML = "";
    jest.restoreAllMocks();
  });

  test("reports not-found when the selector matches nothing", () => {
    document.body.innerHTML = "<div></div>";
    expect(__smLocateForClick("#missing")).toEqual({ found: false });
  });

  test("reports hit:true when elementFromPoint returns the element itself", () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById("btn");
    btn.scrollIntoView = jest.fn();
    btn.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      width: 100,
      height: 20,
    });
    document.elementFromPoint = jest.fn().mockReturnValue(btn);
    Object.defineProperty(window, "innerWidth", {
      value: 800,
      configurable: true,
    });
    Object.defineProperty(window, "innerHeight", {
      value: 600,
      configurable: true,
    });

    const result = __smLocateForClick("#btn");
    expect(result).toMatchObject({
      found: true,
      inViewport: true,
      hit: true,
      cx: 50,
      cy: 10,
    });
  });

  test("does not scroll a target that is already wholly on screen", () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById("btn");
    btn.scrollIntoView = jest.fn();
    btn.getBoundingClientRect = () => ({ left: 10, top: 10, right: 34, bottom: 34, width: 24, height: 24 });
    document.elementFromPoint = jest.fn().mockReturnValue(btn);
    Object.defineProperty(window, "innerWidth", { value: 800, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 600, configurable: true });

    expect(__smLocateForClick("#btn")).toMatchObject({ hit: true, cx: 22, cy: 22 });
    expect(btn.scrollIntoView).not.toHaveBeenCalled();
  });

  test("scrolls a target inside the viewport that a scrolling list clips out of sight", () => {
    document.body.innerHTML = '<button id="btn"></button><div id="row"></div>';
    const btn = document.getElementById("btn");
    const row = document.getElementById("row");
    let scrolled = false;
    btn.scrollIntoView = jest.fn(() => { scrolled = true; });
    btn.getBoundingClientRect = () => ({ left: 10, top: 500, right: 250, bottom: 532, width: 240, height: 32 });
    document.elementFromPoint = jest.fn(() => (scrolled ? btn : row));
    Object.defineProperty(window, "innerWidth", { value: 800, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 600, configurable: true });

    expect(__smLocateForClick("#btn").hit).toBe(true);
    expect(btn.scrollIntoView).toHaveBeenCalled();
  });

  test("scrolls an off-screen target into view without centering it horizontally", () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById("btn");
    let scrolled = false;
    btn.scrollIntoView = jest.fn(() => { scrolled = true; });
    btn.getBoundingClientRect = () =>
      scrolled
        ? { left: 10, top: 300, right: 34, bottom: 324, width: 24, height: 24 }
        : { left: 10, top: 900, right: 34, bottom: 924, width: 24, height: 24 };
    document.elementFromPoint = jest.fn().mockReturnValue(btn);
    Object.defineProperty(window, "innerWidth", { value: 800, configurable: true });
    Object.defineProperty(window, "innerHeight", { value: 600, configurable: true });

    expect(__smLocateForClick("#btn")).toMatchObject({ inViewport: true, cy: 312 });
    expect(btn.scrollIntoView).toHaveBeenCalledWith({ block: "center", inline: "nearest", behavior: "instant" });
  });

  test("reports hit:false when something else is on top", () => {
    document.body.innerHTML =
      '<button id="btn"></button><div id="overlay"></div>';
    const btn = document.getElementById("btn");
    const overlay = document.getElementById("overlay");
    btn.scrollIntoView = jest.fn();
    btn.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      width: 100,
      height: 20,
    });
    document.elementFromPoint = jest.fn().mockReturnValue(overlay);
    Object.defineProperty(window, "innerWidth", {
      value: 800,
      configurable: true,
    });
    Object.defineProperty(window, "innerHeight", {
      value: 600,
      configurable: true,
    });

    expect(__smLocateForClick("#btn").hit).toBe(false);
  });
});

describe("__smClickBySelector", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("clicks the matched element", () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById("btn");
    const onClick = jest.fn();
    btn.addEventListener("click", onClick);

    __smClickBySelector("#btn");
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  test("does not throw when nothing matches", () => {
    document.body.innerHTML = "<div></div>";
    expect(() => __smClickBySelector("#missing")).not.toThrow();
  });
});

describe("__smReadValueBySelector", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("reads the current value of a form field", () => {
    document.body.innerHTML = '<input id="in" value="hello" />';
    expect(__smReadValueBySelector("#in")).toBe("hello");
  });

  test("returns null for an element with no value property", () => {
    document.body.innerHTML = '<div id="el"></div>';
    expect(__smReadValueBySelector("#el")).toBeNull();
  });

  test("returns null when nothing matches", () => {
    document.body.innerHTML = "<div></div>";
    expect(__smReadValueBySelector("#missing")).toBeNull();
  });
});

describe("__smScrollIntoViewBySelector", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("calls scrollIntoView on the matched element", () => {
    document.body.innerHTML = '<div id="el"></div>';
    const el = document.getElementById("el");
    el.scrollIntoView = jest.fn();

    __smScrollIntoViewBySelector("#el");
    expect(el.scrollIntoView).toHaveBeenCalledWith({
      block: "center",
      behavior: "instant",
    });
  });

  test("fails soft on a syntactically invalid selector", () => {
    document.body.innerHTML = "<div></div>";
    expect(() => __smScrollIntoViewBySelector("[")).not.toThrow();
  });
});

describe("__smSelectorExists", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("true when the selector matches, false otherwise", () => {
    document.body.innerHTML = '<div id="el"></div>';
    expect(__smSelectorExists("#el")).toBe(true);
    expect(__smSelectorExists("#missing")).toBe(false);
  });
});

describe("scroll and location helpers", () => {
  test("__smLocationProtocol reflects the current page", () => {
    expect(__smLocationProtocol()).toBe(location.protocol);
  });

  test("__smScrollPosition reflects window scroll offsets", () => {
    Object.defineProperty(window, "scrollX", { value: 12, configurable: true });
    Object.defineProperty(window, "scrollY", { value: 34, configurable: true });
    expect(__smScrollPosition()).toEqual({ x: 12, y: 34 });
  });

  test("__smScrollToY calls window.scrollTo centered on y", () => {
    window.scrollTo = jest.fn();
    Object.defineProperty(window, "innerHeight", {
      value: 600,
      configurable: true,
    });
    __smScrollToY(1000);
    expect(window.scrollTo).toHaveBeenCalledWith(0, 700);
  });

  test("__smScrollBy calls window.scrollBy with the given deltas", () => {
    window.scrollBy = jest.fn();
    __smScrollBy(5, -10);
    expect(window.scrollBy).toHaveBeenCalledWith(5, -10);
  });
});

describe("__smSelectActive", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("selects an input's whole value and returns it", () => {
    document.body.innerHTML = '<input id="in" value="2026-04-10" />';
    const input = document.getElementById("in");
    input.focus();
    expect(__smSelectActive()).toEqual({ selected: true, value: "2026-04-10" });
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 10]);
  });

  test("selects a textarea's whole value", () => {
    document.body.innerHTML =
      '<textarea id="ta">Reminder scheduling overhaul</textarea>';
    const ta = document.getElementById("ta");
    ta.focus();
    expect(__smSelectActive()).toEqual({
      selected: true,
      value: "Reminder scheduling overhaul",
    });
    expect([ta.selectionStart, ta.selectionEnd]).toEqual([0, 28]);
  });

  test("selects the content of a contenteditable", () => {
    document.body.innerHTML =
      '<div id="ce" contenteditable="true" tabindex="0">hello</div>';
    const ce = document.getElementById("ce");
    ce.focus();
    // jsdom has no isContentEditable; stand in for what Chrome reports.
    Object.defineProperty(ce, "isContentEditable", { value: true });
    expect(__smSelectActive()).toEqual({ selected: true, value: "hello" });
    expect(document.getSelection().toString()).toBe("hello");
  });

  test("reports not selected when nothing editable is focused", () => {
    document.body.innerHTML = '<button id="b">x</button>';
    document.getElementById("b").focus();
    expect(__smSelectActive()).toEqual({ selected: false, value: "" });
  });
});

describe("__smClearActiveElement", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("clears the focused input's value, fires input/change, returns the old value", () => {
    document.body.innerHTML = '<input id="in" value="hello" />';
    const input = document.getElementById("in");
    input.focus();
    const onInput = jest.fn();
    const onChange = jest.fn();
    input.addEventListener("input", onInput);
    input.addEventListener("change", onChange);

    expect(__smClearActiveElement()).toEqual({ ok: true, old: "hello" });

    expect(input.value).toBe("");
    expect(onInput).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  test("clears a textarea (the input setter would throw Illegal invocation on it)", () => {
    document.body.innerHTML =
      '<textarea id="ta">Reminder scheduling overhaul</textarea>';
    const ta = document.getElementById("ta");
    ta.focus();
    const onInput = jest.fn();
    ta.addEventListener("input", onInput);

    expect(__smClearActiveElement()).toEqual({
      ok: true,
      old: "Reminder scheduling overhaul",
    });

    expect(ta.value).toBe("");
    expect(onInput).toHaveBeenCalledTimes(1);
  });

  test("clears a select by resetting it", () => {
    document.body.innerHTML =
      '<select id="s"><option value="a">a</option><option value="b" selected>b</option></select>';
    const sel = document.getElementById("s");
    sel.focus();
    expect(() => __smClearActiveElement()).not.toThrow();
    expect(sel.value).toBe("");
  });

  test("uses the native setter, so a framework that wraps .value still sees the change", () => {
    document.body.innerHTML = '<input id="in" value="old" />';
    const input = document.getElementById("in");
    // A React-style tracker: an own-property override that swallows plain assignment.
    const seen = [];
    Object.defineProperty(input, "value", {
      configurable: true,
      get: () =>
        Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          "value",
        ).get.call(input),
      set: (v) => seen.push(v),
    });
    input.focus();
    __smClearActiveElement();
    expect(seen).toEqual([]); // the override was bypassed
    expect(input.value).toBe("");
  });

  test("clears a contenteditable and fires input", () => {
    document.body.innerHTML =
      '<div id="ce" contenteditable="true" tabindex="0">hello</div>';
    const ce = document.getElementById("ce");
    ce.focus();
    Object.defineProperty(ce, "isContentEditable", { value: true });
    const onInput = jest.fn();
    ce.addEventListener("input", onInput);
    expect(__smClearActiveElement()).toEqual({ ok: true, old: "hello" });
    expect(ce.textContent).toBe("");
    expect(onInput).toHaveBeenCalledTimes(1);
  });

  test("reports, rather than throws, when nothing has focus", () => {
    document.body.innerHTML = '<input id="in" value="keep" />';
    document.activeElement.blur();
    const res = __smClearActiveElement();
    expect(res.ok).toBe(false);
    expect(res.reason).toMatch(/nothing has focus/);
    expect(document.getElementById("in").value).toBe("keep");
  });

  test("reports, rather than throws, when a non-field has focus", () => {
    document.body.innerHTML = '<button id="b">x</button>';
    document.getElementById("b").focus();
    const res = __smClearActiveElement();
    expect(res.ok).toBe(false);
    expect(res.reason).toMatch(/<button>/);
  });
});

// Layout helpers for the drag tests: jsdom has no layout, so each element's box
// is declared and elementFromPoint is answered from a list of rectangles.
function box(el, left, top, width, height) {
  el.getBoundingClientRect = () => ({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
  });
}

function viewport(width, height) {
  Object.defineProperty(window, "innerWidth", {
    value: width,
    configurable: true,
  });
  Object.defineProperty(window, "innerHeight", {
    value: height,
    configurable: true,
  });
}

describe("__smFullyVisible", () => {
  beforeEach(() => viewport(1440, 900));
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("true for a box inside the viewport", () => {
    document.body.innerHTML = '<div id="el"></div>';
    box(document.getElementById("el"), 10, 10, 100, 50);
    expect(__smFullyVisible(document.getElementById("el"))).toBe(true);
  });

  test("false when the box is cut by the viewport edge", () => {
    document.body.innerHTML = '<div id="el"></div>';
    box(document.getElementById("el"), 10, 880, 100, 50);
    expect(__smFullyVisible(document.getElementById("el"))).toBe(false);
  });

  test("false when a scrolling ancestor clips it, though it is inside the viewport", () => {
    document.body.innerHTML =
      '<div id="list" style="overflow-y:auto"><div id="el"></div></div>';
    box(document.getElementById("list"), 0, 100, 300, 600);
    box(document.getElementById("el"), 10, 61, 280, 118); // scrolled under the list's top edge
    expect(__smFullyVisible(document.getElementById("el"))).toBe(false);
  });

  test("true when the clipping ancestor shows all of it", () => {
    document.body.innerHTML =
      '<div id="list" style="overflow-y:auto"><div id="el"></div></div>';
    box(document.getElementById("list"), 0, 100, 300, 600);
    box(document.getElementById("el"), 10, 137, 280, 118);
    expect(__smFullyVisible(document.getElementById("el"))).toBe(true);
  });
});

describe("__smDragPrepare", () => {
  beforeEach(() => viewport(1440, 900));
  afterEach(() => {
    document.body.innerHTML = "";
    jest.restoreAllMocks();
  });

  function card() {
    document.body.innerHTML =
      '<div id="list" style="overflow-y:auto"><div id="card"><button id="chip"></button></div></div>';
    const el = document.getElementById("card");
    box(document.getElementById("list"), 0, 100, 400, 700);
    box(el, 20, 527, 300, 118);
    el.scrollIntoView = jest.fn();
    return el;
  }

  test("does not scroll a source that is already fully visible", () => {
    const el = card();
    document.elementFromPoint = jest.fn().mockReturnValue(el);
    __smDragPrepare("#card", null, null);
    expect(el.scrollIntoView).not.toHaveBeenCalled();
  });

  test("scrolls a source that is clipped by its list", () => {
    const el = card();
    box(el, 20, 61, 300, 118);
    document.elementFromPoint = jest.fn().mockReturnValue(el);
    __smDragPrepare("#card", null, null);
    expect(el.scrollIntoView).toHaveBeenCalledWith({
      block: "center",
      inline: "center",
      behavior: "instant",
    });
  });

  test("force scrolls a source that is fully visible in its own viewport", () => {
    const el = card();
    document.elementFromPoint = jest.fn().mockReturnValue(el);
    __smDragPrepare("#card", null, null, true);
    expect(el.scrollIntoView).toHaveBeenCalledTimes(1);
  });

  test("scrolls a source that is below the viewport", () => {
    const el = card();
    box(el, 20, 950, 300, 118);
    document.elementFromPoint = jest.fn().mockReturnValue(el);
    __smDragPrepare("#card", null, null);
    expect(el.scrollIntoView).toHaveBeenCalledTimes(1);
  });

  test("presses on the source, not on an interactive child", () => {
    const el = card();
    const chip = document.getElementById("chip");
    // The center of the card is the chip; everywhere else is the card itself.
    document.elementFromPoint = jest.fn((x, y) =>
      x === 170 && y === 586 ? chip : el,
    );
    const p = __smDragPrepare("#card", null, null);
    expect(p.hit).toBe(true);
    expect([p.x, p.y]).not.toEqual([170, 586]);
    expect(p.view).toEqual({ width: 1440, height: 900 });
  });

  test("an offset is used as given and reports whether the source is there", () => {
    const el = card();
    const other = document.createElement("div");
    document.elementFromPoint = jest.fn((x) => (x < 100 ? el : other));
    expect(__smDragPrepare("#card", 10, 20)).toMatchObject({
      x: 30,
      y: 547,
      hit: true,
    });
    expect(__smDragPrepare("#card", 200, 20)).toMatchObject({
      x: 220,
      y: 547,
      hit: false,
    });
  });

  test("returns null when the element is gone", () => {
    document.body.innerHTML = "<div></div>";
    expect(__smDragPrepare("#missing", null, null)).toBeNull();
  });
});

describe("__smDropCheck", () => {
  beforeEach(() => viewport(1440, 900));
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("hit when the target is top-most at the point", () => {
    document.body.innerHTML = '<div id="t"><span id="inner"></span></div>';
    document.elementFromPoint = jest
      .fn()
      .mockReturnValue(document.getElementById("inner"));
    expect(__smDropCheck("#t", 600, 150)).toEqual({
      hit: true,
      covered: "",
    });
  });

  test("not a hit when a header covers the point, and says what", () => {
    document.body.innerHTML =
      '<div id="t"></div><header data-component="ColumnHeader"><b id="title"></b></header>';
    document.elementFromPoint = jest
      .fn()
      .mockReturnValue(document.getElementById("title"));
    expect(__smDropCheck("#t", 600, 65)).toEqual({
      hit: false,
      covered: "b[data-component=ColumnHeader]",
    });
  });

  test("not a hit when nothing is there", () => {
    document.body.innerHTML = '<div id="t"></div>';
    document.elementFromPoint = jest.fn().mockReturnValue(null);
    expect(__smDropCheck("#t", 600, 65)).toMatchObject({
      hit: false,
      covered: "",
    });
  });

  test("not in the viewport", () => {
    document.body.innerHTML = '<div id="t"></div>';
    expect(__smDropCheck("#t", 600, 5000)).toEqual({
      hit: false,
      covered: "",
    });
  });
});

describe("__smFocusProblem", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("is null when a field has focus", () => {
    document.body.innerHTML = '<input id="in">';
    document.getElementById("in").focus();
    expect(__smFocusProblem()).toBeNull();
  });

  test("says nothing has focus when the page itself does", () => {
    document.body.innerHTML = '<input id="in">';
    document.activeElement.blur();
    expect(__smFocusProblem().nothing).toBe(true);
  });

  test("says another kind of element has focus, without calling it nothing", () => {
    document.body.innerHTML = '<button id="b">x</button>';
    document.getElementById("b").focus();
    const p = __smFocusProblem();
    expect(p.nothing).toBe(false);
    expect(p.reason).toMatch(/not an input or textarea/);
  });
});

describe("__smSetValueIfNative", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test.each([
    ["date", "2027-04-16"],
    ["time", "14:45"],
    ["datetime-local", "2027-04-16T14:45"],
    ["month", "2027-04"],
    ["week", "2027-W15"],
    ["color", "#ff0000"],
    ["range", "7"],
  ])("sets a %s input natively and fires input then change", (type, value) => {
    document.body.innerHTML = `<input id="in" type="${type}">`;
    const input = document.getElementById("in");
    const seen = [];
    input.addEventListener("input", () => seen.push("input"));
    input.addEventListener("change", () => seen.push("change"));

    expect(__smSetValueIfNative("#in", value)).toEqual({
      native: true,
      type,
      blocked: false,
      value,
    });
    expect(input.value).toBe(value);
    expect(seen).toEqual(["input", "change"]);
  });

  test("leaves text-like inputs, textareas and other elements to typing", () => {
    document.body.innerHTML =
      '<input id="t" type="text"><input id="e" type="email"><textarea id="a"></textarea><div id="d"></div>';
    for (const id of ["t", "e", "a", "d"]) {
      expect(__smSetValueIfNative("#" + id, "x")).toEqual({ native: false });
    }
    expect(document.getElementById("t").value).toBe("");
    expect(__smSetValueIfNative("#missing", "x")).toEqual({ native: false });
  });

  test("reports the sanitized value when the control cannot take it", () => {
    document.body.innerHTML = '<input id="in" type="date" value="2027-04-09">';
    const res = __smSetValueIfNative("#in", "next tuesday");
    expect(res.native).toBe(true);
    expect(res.value).toBe("");
  });

  test("refuses a disabled or read-only control", () => {
    document.body.innerHTML =
      '<input id="d" type="date" disabled value="2027-04-09"><input id="r" type="date" readonly value="2027-04-09">';
    expect(__smSetValueIfNative("#d", "2028-01-01").blocked).toBe(true);
    expect(__smSetValueIfNative("#r", "2028-01-01").blocked).toBe(true);
    expect(document.getElementById("d").value).toBe("2027-04-09");
  });

  test("works through React-style value tracking by using the prototype setter", () => {
    document.body.innerHTML = '<input id="in" type="date">';
    const input = document.getElementById("in");
    // A framework shadows the instance's value property; an assignment to it is
    // swallowed, the prototype setter is not.
    Object.defineProperty(input, "value", { set() {}, get: () => "stuck" });
    __smSetValueIfNative("#in", "2027-04-16");
    const proto = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    );
    expect(proto.get.call(input)).toBe("2027-04-16");
  });
});

describe("__smFocusBySelector", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("scrolls to and focuses the element, reporting that it exists", () => {
    document.body.innerHTML = '<input id="in">';
    const el = document.getElementById("in");
    el.scrollIntoView = jest.fn();
    expect(__smFocusBySelector("#in")).toBe(true);
    expect(el.scrollIntoView).toHaveBeenCalled();
    expect(document.activeElement).toBe(el);
  });

  test("returns false when nothing matches", () => {
    expect(__smFocusBySelector("#missing")).toBe(false);
  });
});
