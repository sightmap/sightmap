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
// has passed. It watches the document with a MutationObserver (subtree, child
// lists, attributes, text) and also waits for the page's fetches in flight (see
// trackInflight), so a list that renders after an /api response is not taken as
// settled while the response is pending.
//
// It is the condition-free wait an agent wants after an action ("let the page
// finish reacting") where it used to sleep a fixed time; a wait with a
// condition is `expect`.
func WaitSettled(ctx context.Context, conn *CDPConn, quiet, max time.Duration) (SettleResult, error) {
	script := fmt.Sprintf(`new Promise((resolve) => {
  const quiet = %d, max = %d, start = performance.now();
  let last = start;
  const obs = new MutationObserver(() => { last = performance.now(); });
  obs.observe(document, { subtree: true, childList: true, attributes: true, characterData: true });
  const pending = () => window.__smInflight || 0;
  const tick = () => {
    const now = performance.now();
    if (now - last >= quiet && pending() === 0) { obs.disconnect(); resolve({ ms: Math.round(now - start), quiet: true }); return; }
    if (now - start >= max) { obs.disconnect(); resolve({ ms: Math.round(now - start), quiet: false }); return; }
    setTimeout(tick, 25);
  };
  setTimeout(tick, 25);
})`, quiet.Milliseconds(), max.Milliseconds())
	ctx, cancel := context.WithTimeout(ctx, max+5*time.Second)
	defer cancel()
	if err := trackInflight(ctx, conn); err != nil {
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

// trackInflight installs, once per document, a counter of fetches in flight on
// window.__smInflight, by wrapping window.fetch and XMLHttpRequest.send. It is
// idempotent: a document that already has it is left alone. Requests started
// before it was installed are not counted; WaitSettled's DOM quiet period covers
// those.
func trackInflight(ctx context.Context, conn *CDPConn) error {
	_, err := EvalJSON(ctx, conn, `(() => {
  if (window.__smInflightInstalled) return true;
  window.__smInflightInstalled = true;
  window.__smInflight = 0;
  const inc = () => { window.__smInflight++; }, dec = () => { window.__smInflight = Math.max(0, window.__smInflight - 1); };
  const f = window.fetch;
  if (f) window.fetch = function (...a) { inc(); return f.apply(this, a).finally(dec); };
  const send = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.send = function (...a) { inc(); this.addEventListener('loadend', dec, { once: true }); return send.apply(this, a); };
  return true;
})()`)
	if err != nil {
		return fmt.Errorf("WaitSettled: track requests: %w", err)
	}
	return nil
}
