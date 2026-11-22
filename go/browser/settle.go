package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SettleResult says how a WaitSettled ended: how long it took, and whether the
// page went quiet or the cap ran out with the DOM still changing.
type SettleResult struct {
	Elapsed time.Duration
	Quiet   bool
}

// WaitSettled waits until the page's DOM has not changed for quiet, or until max
// has passed. It watches the document and every open shadow root with a
// MutationObserver (subtree, child lists, attributes, text), since a
// document-level observer does not see inside shadow trees. A shadow root that
// appears while waiting is observed too: inside an added subtree, or attached to
// an existing element (attachShadow makes no mutation record, so instrument hooks
// it). It also waits for the page's fetches in flight (see instrument), so a list
// that renders after an /api response is not taken as settled while the response
// is pending.
//
// A caller that acts first should call ArmSettle before the action: a request
// the action starts is only counted once the tracker is installed.
//
// The observers disconnect themselves at max even if the caller has gone (the
// eval errored or was cancelled), so repeated waits never pile observers up on a
// document.
//
// It is the condition-free wait an agent wants after an action ("let the page
// finish reacting") where it used to sleep a fixed time; a wait with a
// condition is `expect`.
func WaitSettled(ctx context.Context, conn *CDPConn, quiet, max time.Duration) (SettleResult, error) {
	script := fmt.Sprintf(`new Promise((resolve) => {
  const quiet = %d, max = %d, start = performance.now();
  const opts = { subtree: true, childList: true, attributes: true, characterData: true };
  let last = start, done = false;
  const seen = new WeakSet();
  // Observe a root (the document or a shadow root) and the open shadow roots inside it.
  const watch = (root) => {
    if (seen.has(root)) return;
    seen.add(root);
    obs.observe(root, opts);
    shadowsIn(root);
  };
  const shadowsIn = (node) => {
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_ELEMENT);
    for (let n = walker.currentNode; n; n = walker.nextNode()) if (n.shadowRoot) watch(n.shadowRoot);
  };
  const obs = new MutationObserver((records) => {
    last = performance.now();
    for (const r of records) for (const n of r.addedNodes) if (n.nodeType === 1) shadowsIn(n);
  });
  const finish = (q) => {
    if (done) return;
    done = true;
    obs.disconnect();
    window.__smOnShadow = null;
    resolve({ ms: Math.round(performance.now() - start), quiet: q });
  };
  watch(document);
  window.__smOnShadow = (root) => { if (!done) { last = performance.now(); watch(root); } };
  const pending = () => window.__smInflight || 0;
  const tick = () => {
    if (done) return;
    const now = performance.now();
    if (now - last >= quiet && pending() === 0) return finish(true);
    if (now - start >= max) return finish(false);
    setTimeout(tick, 25);
  };
  setTimeout(tick, 25);
  setTimeout(() => finish(false), max + 50);
})`, quiet.Milliseconds(), max.Milliseconds())
	ctx, cancel := context.WithTimeout(ctx, max+5*time.Second)
	defer cancel()
	if err := instrument(ctx, conn); err != nil {
		return SettleResult{}, err
	}
	raw, err := EvalJSON(ctx, conn, script)
	if err != nil {
		return SettleResult{}, fmt.Errorf("WaitSettled: %w", err)
	}
	var r struct {
		Ms    int64 `json:"ms"`
		Quiet bool  `json:"quiet"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return SettleResult{}, fmt.Errorf("WaitSettled: %w", err)
	}
	return SettleResult{Elapsed: time.Duration(r.Ms) * time.Millisecond, Quiet: r.Quiet}, nil
}

// ArmSettle installs what WaitSettled reads (see instrument). Call it before an
// action whose reaction WaitSettled will wait for, so a request the action
// starts is counted from the moment it is sent. Without it the counter is
// installed inside WaitSettled, after the action, and a slow request the action
// started is invisible: the wait ends once the DOM has been quiet for the quiet
// period, with the response still pending. It does nothing on a document that is
// already armed, and a navigation drops it (WaitSettled installs it again).
func ArmSettle(ctx context.Context, conn *CDPConn) error {
	return instrument(ctx, conn)
}

// instrument installs, once per document, what WaitSettled needs to see beyond a
// MutationObserver: a counter of fetches in flight on window.__smInflight
// (wrapping window.fetch and XMLHttpRequest.send), and a hook on attachShadow
// that hands each new shadow root to the wait in progress (window.__smOnShadow).
// It is idempotent: a document that already has it is left alone. Requests
// started before it was installed are not counted; WaitSettled's DOM quiet period
// covers those.
func instrument(ctx context.Context, conn *CDPConn) error {
	_, err := EvalJSON(ctx, conn, `(() => {
  if (window.__smSettleInstalled) return true;
  window.__smSettleInstalled = true;
  window.__smInflight = 0;
  const attach = Element.prototype.attachShadow;
  Element.prototype.attachShadow = function (...a) {
    const root = attach.apply(this, a);
    if (window.__smOnShadow && root.mode === 'open') window.__smOnShadow(root);
    return root;
  };
  const inc = () => { window.__smInflight++; }, dec = () => { window.__smInflight = Math.max(0, window.__smInflight - 1); };
  const f = window.fetch;
  if (f) window.fetch = function (...a) { inc(); return f.apply(this, a).finally(dec); };
  const send = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.send = function (...a) {
    inc();
    this.addEventListener('loadend', dec, { once: true });
    try { return send.apply(this, a); } catch (e) { dec(); throw e; }
  };
  return true;
})()`)
	if err != nil {
		return fmt.Errorf("WaitSettled: track requests: %w", err)
	}
	return nil
}
