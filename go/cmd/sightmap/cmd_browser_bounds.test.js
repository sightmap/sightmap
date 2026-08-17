const fs = require('fs');
const path = require('path');

(0, eval)(fs.readFileSync(path.join(__dirname, '..', '..', 'browser', 'deepquery.js'), 'utf8'));
(0, eval)(fs.readFileSync(path.join(__dirname, 'cmd_browser_bounds.js'), 'utf8'));

describe('__smBoundsBySelector', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  test('returns a bounds+label entry per match, preferring aria-label over text', () => {
    document.body.innerHTML = `
      <button id="a" aria-label="Save changes">Save</button>
      <button id="b">  Cancel   now  </button>
    `;
    document.getElementById('a').getBoundingClientRect = () => ({left: 0, top: 0, width: 10, height: 5});
    document.getElementById('b').getBoundingClientRect = () => ({left: 1, top: 2, width: 3, height: 4});

    const results = __smBoundsBySelector('button');
    expect(results).toEqual([
      {x: 0, y: 0, width: 10, height: 5, label: 'Save changes'},
      {x: 1, y: 2, width: 3, height: 4, label: 'Cancel now'},
    ]);
  });

  test('truncates an overlong label to 80 characters', () => {
    document.body.innerHTML = `<button id="a">${'x'.repeat(200)}</button>`;
    document.getElementById('a').getBoundingClientRect = () => ({left: 0, top: 0, width: 1, height: 1});

    expect(__smBoundsBySelector('button')[0].label).toHaveLength(80);
  });

  test('finds matches inside shadow DOM', () => {
    const host = document.createElement('div');
    document.body.appendChild(host);
    const btn = document.createElement('button');
    btn.getBoundingClientRect = () => ({left: 0, top: 0, width: 1, height: 1});
    host.attachShadow({mode: 'open'}).appendChild(btn);

    expect(__smBoundsBySelector('button')).toHaveLength(1);
  });

  test('returns an empty list when nothing matches', () => {
    document.body.innerHTML = '<div></div>';
    expect(__smBoundsBySelector('button')).toEqual([]);
  });
});

describe('__smViewportSize', () => {
  test('reads window.innerWidth/innerHeight', () => {
    Object.defineProperty(window, 'innerWidth', {value: 1024, configurable: true});
    Object.defineProperty(window, 'innerHeight', {value: 768, configurable: true});
    expect(__smViewportSize()).toEqual({w: 1024, h: 768});
  });
});
