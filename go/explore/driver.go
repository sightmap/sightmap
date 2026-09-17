package explore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Driver is what the loop needs from a browser. CDPDriver is the real one; tests
// use a fake.
type Driver interface {
	Observe(ctx context.Context) (*Page, error)
	URL(ctx context.Context) (string, error)
	Click(ctx context.Context, n *Node) error
	Fill(ctx context.Context, n *Node, value string) error
	SelectOptions(ctx context.Context, n *Node) ([]string, error)
	Select(ctx context.Context, n *Node, index int) error
	Back(ctx context.Context) error
	Scroll(ctx context.Context) error
	// Settle waits for the page to stop changing after an action.
	Settle(ctx context.Context, urlBefore string) SettleInfo
	Navigate(ctx context.Context, url string) error
	ClearStorage(ctx context.Context) error
}

// CDPDriver drives one Chrome tab over an open CDP connection, keeping the
// connection and corpus for the whole run so a step costs no process spawn.
type CDPDriver struct {
	Conn        *browser.CDPConn
	Corpus      *sightmap.Corpus // nil when the site has no corpus
	VisibleOnly bool
	Settling    SettleOptions
}

// NewCDPDriver wraps an open connection.
func NewCDPDriver(conn *browser.CDPConn, corpus *sightmap.Corpus) *CDPDriver {
	return &CDPDriver{Conn: conn, Corpus: corpus, VisibleOnly: true}
}

// Observe extracts and matches the current page. Every call re-stamps the
// data-sightmap-id attributes, so act on nodes from the same Page.
func (d *CDPDriver) Observe(ctx context.Context) (*Page, error) {
	res, err := observe.Page(ctx, d.Conn, d.Corpus, observe.Options{VisibleOnly: d.VisibleOnly})
	if err != nil {
		return nil, err
	}
	u := res.URL
	if u == "" {
		u, _ = browser.GetURL(ctx, d.Conn)
	}
	return NewPage(res, u), nil
}

func (d *CDPDriver) URL(ctx context.Context) (string, error) { return browser.GetURL(ctx, d.Conn) }

// Click scrolls the element stamped with the node's data-sightmap-id into
// view and dispatches a DOM click (a few milliseconds). When the element is
// gone it reports a stale element; when the DOM click throws it falls back to
// the real mouse path (scroll, hit-test, Input.dispatchMouseEvent).
func (d *CDPDriver) Click(ctx context.Context, n *Node) error {
	res, err := browser.EvalJSON(ctx, d.Conn, jsByID(n.ID, `try{el.scrollIntoView({block:'center',inline:'nearest'});}catch(e){} el.click(); return 'ok';`))
	if err == nil {
		if string(res) == `"noel"` {
			return fmt.Errorf("element %s not found in live DOM", n.ID)
		}
		return nil
	}
	if _, _, mErr := browser.Click(ctx, d.Conn, n.Raw); mErr != nil {
		return fmt.Errorf("click %s: %v (mouse fallback: %v)", n.ID, err, mErr)
	}
	return nil
}

// Fill sets the value through the native setter and dispatches input and
// change events, which framework-controlled inputs honour, then verifies. If
// the value does not stick it falls back to real key events.
func (d *CDPDriver) Fill(ctx context.Context, n *Node, value string) error {
	v, _ := json.Marshal(value)
	res, err := browser.EvalJSON(ctx, d.Conn, jsByID(n.ID, `el.focus();var p=el.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;var dsc=Object.getOwnPropertyDescriptor(p,'value');if(dsc&&dsc.set){dsc.set.call(el,`+string(v)+`);}else{el.value=`+string(v)+`;}el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));return el.value;`))
	if err == nil {
		if string(res) == `"noel"` {
			return fmt.Errorf("element %s not found in live DOM", n.ID)
		}
		var got string
		_ = json.Unmarshal(res, &got)
		if got == value {
			return nil
		}
	}
	if kErr := browser.ClearAndFill(ctx, d.Conn, n.Raw, value); kErr != nil {
		return fmt.Errorf("fill %s: %v (key fallback: %v)", n.ID, err, kErr)
	}
	return nil
}

func (d *CDPDriver) SelectOptions(ctx context.Context, n *Node) ([]string, error) {
	res, err := browser.EvalJSON(ctx, d.Conn, jsByID(n.ID, `if(!el.options)return [];return Array.from(el.options).map(function(o){return o.text;});`))
	if err != nil {
		return nil, err
	}
	var opts []string
	if err := json.Unmarshal(res, &opts); err != nil {
		return nil, nil
	}
	return opts, nil
}

func (d *CDPDriver) Select(ctx context.Context, n *Node, index int) error {
	res, err := browser.EvalJSON(ctx, d.Conn, jsByID(n.ID, fmt.Sprintf(`if(!el.options)return 'noel';el.selectedIndex=%d;el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));return el.value;`, index)))
	if err != nil {
		return err
	}
	if string(res) == `"noel"` {
		return fmt.Errorf("element %s not found in live DOM", n.ID)
	}
	return nil
}

func (d *CDPDriver) Back(ctx context.Context) error {
	_, err := browser.EvalJSON(ctx, d.Conn, `(history.back(), "back")`)
	return err
}

func (d *CDPDriver) Scroll(ctx context.Context) error {
	return browser.ScrollBy(ctx, d.Conn, 0, 600)
}

func (d *CDPDriver) Settle(ctx context.Context, urlBefore string) SettleInfo {
	return Settle(ctx, d.Conn, urlBefore, d.Settling)
}

func (d *CDPDriver) Navigate(ctx context.Context, target string) error {
	return browser.NavigateAndWaitIdle(ctx, d.Conn, target, 8*time.Second)
}

// ClearStorage wipes cookies and the current origin's storage so a goal starts
// from a clean session.
func (d *CDPDriver) ClearStorage(ctx context.Context) error {
	if err := browser.ClearBrowserCookies(ctx, d.Conn); err != nil {
		return err
	}
	u, err := browser.GetURL(ctx, d.Conn)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || !strings.HasPrefix(parsed.Scheme, "http") {
		return nil
	}
	return browser.ClearStorageForOrigin(ctx, d.Conn, parsed.Scheme+"://"+parsed.Host)
}

// jsByID wraps body in a function that first resolves the element by
// data-sightmap-id, returning "noel" when it is gone.
func jsByID(id, body string) string {
	idJSON, _ := json.Marshal(id)
	return `(function(){var el=document.querySelector('[data-sightmap-id='+JSON.stringify(` + string(idJSON) + `)+']');if(!el)return 'noel';` + body + `})()`
}
