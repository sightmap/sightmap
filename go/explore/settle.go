package explore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

// settleScript installs a MutationObserver counter once per document and
// returns [href, readyState, mutationCount, bodyTextLength]. A navigation
// replaces the window, so the counter reinstalls and restarts at 0, which the
// sampler sees as change.
const settleScript = `(function(){
if(!window.__smSettle){var n=0;try{new MutationObserver(function(){n++;}).observe(document.documentElement,{subtree:true,childList:true,attributes:true,characterData:true});}catch(e){}window.__smSettle=function(){return n;};}
return [location.href, document.readyState, window.__smSettle(), (document.body&&document.body.innerText||'').length];})()`

// sample is one reading of the page's settle signals.
type sample struct {
	URL        string
	ReadyState string
	Mutations  int
	TextLen    int
}

// SettleOptions tunes the wait after an action.
type SettleOptions struct {
	Poll   time.Duration // interval between samples (default 40 ms)
	Max    time.Duration // give up after this (default 3 s)
	Coarse time.Duration // after this long, ignore the mutation counter and require only URL, readyState, and text length to be stable (default 1 s)
}

// SettleInfo reports what the wait observed.
type SettleInfo struct {
	Ms        int
	URL       string
	Navigated bool // the URL changed from before the action
	TimedOut  bool
}

// quiet decides whether two consecutive samples mean the page has settled.
// The mutation counter is ignored once elapsed passes coarse, so a page with a
// permanent animation still settles on the coarser signals.
func quiet(prev, cur sample, elapsed, coarse time.Duration) bool {
	if cur.ReadyState != "complete" {
		return false
	}
	if cur.URL != prev.URL || cur.TextLen != prev.TextLen {
		return false
	}
	if elapsed < coarse && cur.Mutations != prev.Mutations {
		return false
	}
	return true
}

// Settle waits until the page stops changing after an action: readyState is
// complete and two consecutive samples agree on URL, mutation count, and text
// length. urlBefore lets the caller learn whether the action navigated.
func Settle(ctx context.Context, conn *browser.CDPConn, urlBefore string, opts SettleOptions) SettleInfo {
	if opts.Poll <= 0 {
		opts.Poll = 40 * time.Millisecond
	}
	if opts.Max <= 0 {
		opts.Max = 3 * time.Second
	}
	if opts.Coarse <= 0 {
		opts.Coarse = time.Second
	}
	t0 := time.Now()
	var prev *sample
	info := SettleInfo{}
	for {
		sleepCtx(ctx, opts.Poll)
		elapsed := time.Since(t0)
		cur, err := readSample(ctx, conn)
		if err == nil {
			info.URL = cur.URL
			info.Navigated = cur.URL != urlBefore
			if prev != nil && quiet(*prev, cur, elapsed, opts.Coarse) {
				info.Ms = int(elapsed.Milliseconds())
				return info
			}
			prev = &cur
		} else {
			// Mid-navigation evals fail; treat as change and keep sampling.
			prev = nil
		}
		if elapsed >= opts.Max || ctx.Err() != nil {
			info.Ms = int(elapsed.Milliseconds())
			info.TimedOut = true
			if info.URL == "" {
				if u, err := browser.GetURL(ctx, conn); err == nil {
					info.URL = u
					info.Navigated = u != urlBefore
				}
			}
			return info
		}
	}
}

func readSample(ctx context.Context, conn *browser.CDPConn) (sample, error) {
	raw, err := browser.EvalJSON(ctx, conn, settleScript)
	if err != nil {
		return sample{}, err
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return sample{}, err
	}
	if len(arr) < 4 {
		return sample{}, errShortSample
	}
	var s sample
	_ = json.Unmarshal(arr[0], &s.URL)
	_ = json.Unmarshal(arr[1], &s.ReadyState)
	_ = json.Unmarshal(arr[2], &s.Mutations)
	_ = json.Unmarshal(arr[3], &s.TextLen)
	return s, nil
}

type settleError string

func (e settleError) Error() string { return string(e) }

const errShortSample = settleError("settle: short sample")
