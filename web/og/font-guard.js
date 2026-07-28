// Verifies each required font face actually loaded — not just that its
// network request settled — before a card can be considered safe to
// screenshot. If any face failed, stamps an unmissable red banner across the
// card so a blocked or slow Google Fonts fetch can never silently produce a
// wrong-typeface PNG that exits 0 with no signal.
//
// `document.fonts.check()` is NOT sufficient here — verified empirically:
// when the stylesheet fails to load, zero FontFace entries are ever
// registered in document.fonts, and per the CSS Font Loading Module spec,
// check() optimistically returns true when it has no @font-face declaration
// to contradict it, regardless of whether the named family is actually
// available. `document.fonts.load()` instead performs a real load attempt
// and resolves an empty FontFace[] when it fails, so that's what this uses.
async function assertFontsLoaded(specs) {
  const results = await Promise.all(
    specs.map((css) =>
      document.fonts.load(css).then(
        (faces) => faces.length > 0,
        () => false
      )
    )
  );
  if (results.every(Boolean)) return;

  const banner = document.createElement('div');
  banner.textContent = 'FONTS FAILED TO LOAD — DO NOT COMMIT THIS CARD';
  banner.style.cssText = [
    'position:fixed', 'left:0', 'top:0', 'width:100%', 'box-sizing:border-box',
    'padding:26px 0', 'margin:0', 'background:#ff0000', 'color:#ffffff',
    'font-family:system-ui,sans-serif', 'font-weight:900', 'font-size:36px',
    'text-align:center', 'letter-spacing:0.02em', 'z-index:2147483647',
  ].join(';');
  document.body.appendChild(banner);
}
