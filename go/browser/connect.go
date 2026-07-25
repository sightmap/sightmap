package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

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
