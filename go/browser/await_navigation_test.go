package browser

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// AwaitNavigation returns (url, true) when a navigation event fires within the
// window. The fake CDP emits Page.frameNavigated shortly after Page.navigate, so
// triggering a navigate after AwaitNavigation has subscribed exercises the path.
func TestAwaitNavigation_DetectsNavigation(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		return json.RawMessage(`{}`)
	})
	ctx := context.Background()

	// Kick a navigate slightly after AwaitNavigation subscribes; the fake fires
	// Page.frameNavigated 50 ms after that.
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = Navigate(ctx, conn, "about:blank")
	}()

	_, moved := AwaitNavigation(ctx, conn, 2*time.Second)
	if !moved {
		t.Fatal("expected AwaitNavigation to report a navigation")
	}
}

// With no navigation, AwaitNavigation returns ("", false) once maxWait elapses.
func TestAwaitNavigation_TimesOutWhenStill(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		return json.RawMessage(`{}`)
	})
	start := time.Now()
	url, moved := AwaitNavigation(context.Background(), conn, 200*time.Millisecond)
	if moved || url != "" {
		t.Fatalf("expected no navigation, got url=%q moved=%v", url, moved)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("returned too early (%v); should wait out maxWait", elapsed)
	}
}
