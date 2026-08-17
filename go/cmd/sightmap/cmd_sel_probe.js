// Live-DOM helpers for the `sel-probe` subcommand. Depends on
// __smDeepQueryAll from browser/deepquery.js being prepended.

function __smQueryElements(sel, max, full) {
  try {
    const els = [...__smDeepQueryAll(document, sel)].slice(0, max);
    const interestingAttrs = new Set([
      'id', 'class', 'role', 'href', 'type', 'name', 'placeholder', 'aria-label',
      'aria-expanded', 'aria-haspopup', 'aria-controls', 'aria-selected',
      'data-testid', 'data-component', 'tabindex'
    ]);
    return els.map(el => {
      const attrs = {};
      for (const a of el.attributes) {
        if (interestingAttrs.has(a.name) || a.name.startsWith('aria-') || a.name.startsWith('data-')) {
          if (a.value !== '') attrs[a.name] = a.value;
        }
      }
      const parents = [];
      let p = el.parentElement;
      for (let i = 0; i < 5 && p && p.tagName !== 'HTML'; i++, p = p.parentElement) {
        parents.push({
          tag: p.tagName.toLowerCase(),
          id:  p.id || '',
          cls: [...p.classList].slice(0, 4),
          dt:  p.getAttribute('data-testid') || '',
          dc:  p.getAttribute('data-component') || '',
        });
      }
      return {
        tag:     el.tagName.toLowerCase(),
        id:      el.id || '',
        cls:     [...el.classList],
        role:    el.getAttribute('role') || '',
        text:    full ? el.textContent.trim() : el.textContent.trim().slice(0, 80),
        attrs,
        parents,
      };
    });
  } catch(e) {
    return {error: e.message};
  }
}

function __smLiveSelectorCount(sel) {
  try {
    return __smDeepQueryAll(document, sel).length;
  } catch(e) {
    return -1;
  }
}

function __smSelProbeAllCount(sel, max) {
  try {
    return Array.from(__smDeepQueryAll(document, sel)).slice(0, max).length;
  } catch(e) {
    return -1;
  }
}
