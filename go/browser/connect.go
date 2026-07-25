package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ConnectOrLaunch attaches to a running Chrome session, or launches a fresh
// headed one when launch is true and no session is reachable. It returns the
// connection and a cleanup func the caller must defer: Close for an attached
// session, or full process teardown for a launched one. When launch is false and
// nothing is reachable, the connect error is returned unwrapped so callers can
// present their own guidance. This is the single connection primitive every
// page-affecting command (and external callers that own the browser lifecycle)
// should build on.
func ConnectOrLaunch(ctx context.Context, addr, tabID string, launch bool) (*CDPConn, func(), error) {
	conn, err := Connect(addr, tabID)
	if err == nil {
		return conn, func() { conn.Close() }, nil
	}
	if !launch {
		return nil, nil, err
	}
	lconn, cleanup, lerr := Launch(ctx, LaunchOptions{})
	if lerr != nil {
		return nil, nil, fmt.Errorf("cannot launch Chrome: %w", lerr)
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	return lconn, cleanup, nil
}

// Connect attaches to a running Chrome session for a page-affecting command.
// With an explicit tabID it connects to that tab. Without one it auto-selects
// the single open CONTENT tab, but errors (listing the tabs) when there are zero
// or several — so concurrent agents can never silently drive each other's tab,
// and a command never lands on the extension side panel. Callers own the
// returned connection and must Close it.
func Connect(addr, tabID string) (*CDPConn, error) {
	ctx := context.Background()
	if tabID != "" {
		conn, err := ConnectTab(ctx, addr, tabID)
		if err != nil {
			return nil, fmt.Errorf("cannot connect to tab %s at %s\n"+
				"Run 'sightmap browser tabs list' to see open tabs.", tabID, addr)
		}
		return conn, nil
	}

	tabs, err := ListTabs(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Chrome at %s\n"+
			"Start a session first: sightmap browser start", addr)
	}
	switch len(tabs) {
	case 0:
		return nil, fmt.Errorf("no content tab open at %s\n"+
			"Start a session first: sightmap browser start", addr)
	case 1:
		conn, err := ConnectTab(ctx, addr, tabs[0].TargetID)
		if err != nil {
			return nil, fmt.Errorf("cannot connect to tab %s at %s", tabs[0].TargetID, addr)
		}
		return conn, nil
	default:
		return nil, AmbiguousTabError(tabs)
	}
}

// AmbiguousTabError builds the "pass --tab" guidance shown when several content
// tabs are open (the concurrent multi-agent case).
func AmbiguousTabError(tabs []TabInfo) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d content tabs are open — pass --tab <ID> to choose one\n", len(tabs))
	b.WriteString("(no default when a session has multiple tabs, to avoid cross-agent crosstalk):\n")
	for _, t := range tabs {
		url := t.URL
		if len(url) > 70 {
			url = url[:67] + "..."
		}
		fmt.Fprintf(&b, "  --tab %s  %s\n", t.TargetID, url)
	}
	b.WriteString("Your session's tab ID was printed by 'sightmap browser start'.")
	return errors.New(b.String())
}
