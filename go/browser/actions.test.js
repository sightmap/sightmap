const fs = require('fs');
const path = require('path');

(0, eval)(fs.readFileSync(path.join(__dirname, 'deepquery.js'), 'utf8'));
(0, eval)(fs.readFileSync(path.join(__dirname, 'actions.js'), 'utf8'));

describe('__smBoundsBySelector', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('returns rounded bounds for the matched element', () => {
    document.body.innerHTML = '<div id="el"></div>';
    const el = document.getElementById('el');
    el.getBoundingClientRect = () => ({left: 1.4, top: 2.6, width: 10.5, height: 20.5});

    expect(__smBoundsBySelector('#el')).toEqual({x: 1, y: 3, width: 11, height: 21});
  });

  test('returns null when nothing matches', () => {
    document.body.innerHTML = '<div></div>';
    expect(__smBoundsBySelector('#missing')).toBeNull();
  });
});

describe('__smLocateForClick', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    jest.restoreAllMocks();
  });

  test('reports not-found when the selector matches nothing', () => {
    document.body.innerHTML = '<div></div>';
    expect(__smLocateForClick('#missing')).toEqual({found: false});
  });

  test('reports hit:true when elementFromPoint returns the element itself', () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById('btn');
    btn.scrollIntoView = jest.fn();
    btn.getBoundingClientRect = () => ({left: 0, top: 0, width: 100, height: 20});
    document.elementFromPoint = jest.fn().mockReturnValue(btn);
    Object.defineProperty(window, 'innerWidth', {value: 800, configurable: true});
    Object.defineProperty(window, 'innerHeight', {value: 600, configurable: true});

    const result = __smLocateForClick('#btn');
    expect(result).toMatchObject({found: true, inViewport: true, hit: true, cx: 50, cy: 10});
  });

  test('reports hit:false when something else is on top', () => {
    document.body.innerHTML = '<button id="btn"></button><div id="overlay"></div>';
    const btn = document.getElementById('btn');
    const overlay = document.getElementById('overlay');
    btn.scrollIntoView = jest.fn();
    btn.getBoundingClientRect = () => ({left: 0, top: 0, width: 100, height: 20});
    document.elementFromPoint = jest.fn().mockReturnValue(overlay);
    Object.defineProperty(window, 'innerWidth', {value: 800, configurable: true});
    Object.defineProperty(window, 'innerHeight', {value: 600, configurable: true});

    expect(__smLocateForClick('#btn').hit).toBe(false);
  });
});

describe('__smClickBySelector', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('clicks the matched element', () => {
    document.body.innerHTML = '<button id="btn"></button>';
    const btn = document.getElementById('btn');
    const onClick = jest.fn();
    btn.addEventListener('click', onClick);

    __smClickBySelector('#btn');
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  test('does not throw when nothing matches', () => {
    document.body.innerHTML = '<div></div>';
    expect(() => __smClickBySelector('#missing')).not.toThrow();
  });
});

describe('__smReadValueBySelector', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('reads the current value of a form field', () => {
    document.body.innerHTML = '<input id="in" value="hello" />';
    expect(__smReadValueBySelector('#in')).toBe('hello');
  });

  test('returns null for an element with no value property', () => {
    document.body.innerHTML = '<div id="el"></div>';
    expect(__smReadValueBySelector('#el')).toBeNull();
  });

  test('returns null when nothing matches', () => {
    document.body.innerHTML = '<div></div>';
    expect(__smReadValueBySelector('#missing')).toBeNull();
  });
});

describe('__smScrollIntoViewBySelector', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('calls scrollIntoView on the matched element', () => {
    document.body.innerHTML = '<div id="el"></div>';
    const el = document.getElementById('el');
    el.scrollIntoView = jest.fn();

    __smScrollIntoViewBySelector('#el');
    expect(el.scrollIntoView).toHaveBeenCalledWith({block: 'center', behavior: 'instant'});
  });

  test('fails soft on a syntactically invalid selector', () => {
    document.body.innerHTML = '<div></div>';
    expect(() => __smScrollIntoViewBySelector('[')).not.toThrow();
  });
});

describe('__smSelectorExists', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('true when the selector matches, false otherwise', () => {
    document.body.innerHTML = '<div id="el"></div>';
    expect(__smSelectorExists('#el')).toBe(true);
    expect(__smSelectorExists('#missing')).toBe(false);
  });
});

describe('scroll and location helpers', () => {
  test('__smLocationProtocol reflects the current page', () => {
    expect(__smLocationProtocol()).toBe(location.protocol);
  });

  test('__smScrollPosition reflects window scroll offsets', () => {
    Object.defineProperty(window, 'scrollX', {value: 12, configurable: true});
    Object.defineProperty(window, 'scrollY', {value: 34, configurable: true});
    expect(__smScrollPosition()).toEqual({x: 12, y: 34});
  });

  test('__smScrollToY calls window.scrollTo centered on y', () => {
    window.scrollTo = jest.fn();
    Object.defineProperty(window, 'innerHeight', {value: 600, configurable: true});
    __smScrollToY(1000);
    expect(window.scrollTo).toHaveBeenCalledWith(0, 700);
  });

  test('__smScrollBy calls window.scrollBy with the given deltas', () => {
    window.scrollBy = jest.fn();
    __smScrollBy(5, -10);
    expect(window.scrollBy).toHaveBeenCalledWith(5, -10);
  });
});

describe('__smClearActiveElement', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test("clears the focused input's value and fires input/change events", () => {
    document.body.innerHTML = '<input id="in" value="hello" />';
    const input = document.getElementById('in');
    input.focus();
    const onInput = jest.fn();
    const onChange = jest.fn();
    input.addEventListener('input', onInput);
    input.addEventListener('change', onChange);

    __smClearActiveElement();

    expect(input.value).toBe('');
    expect(onInput).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});
