package atlas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetch limits. A timeout bounds how long a response may take, not how large
// it may be: 30 seconds of a gigabit link is several gigabytes, and an install
// holds every fetched body in memory at once so it can write them atomically.
// Corpus files are kilobyte-scale YAML, so these caps are generous by orders
// of magnitude and exist only to make a hostile or broken mirror fail fast.
const (
	// MaxIndexBytes caps the atlas index; it lists every published entry.
	MaxIndexBytes = 8 << 20 // 8 MiB
	// MaxFileBytes caps a single corpus file.
	MaxFileBytes = 4 << 20 // 4 MiB
	// MaxEntryBytes caps one entry's files in total.
	MaxEntryBytes = 32 << 20 // 32 MiB
	// MaxRedirects caps the redirect chain of a single fetch.
	MaxRedirects = 5
	// FetchTimeout bounds a whole install's HTTP traffic.
	FetchTimeout = 30 * time.Second
	// FetchConcurrency bounds how many files are fetched at once. One RTT per
	// file is the dominant cost of installing a corpus.
	FetchConcurrency = 6
)

// Client fetches atlas content under a fixed policy: HTTPS only (plain HTTP
// for loopback), that policy re-applied to every redirect hop, a bounded
// redirect chain, a response size cap, and an overall timeout. The redirect
// check is the load-bearing part — without it a mirror or a
// man-in-the-middle answering `302 Location: http://…` downgrades the fetch to
// plaintext after the scheme was already approved.
type Client struct {
	http *http.Client
}

// NewClient returns a Client with the default policy.
func NewClient() *Client {
	return &Client{http: &http.Client{
		Timeout: FetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", MaxRedirects)
			}
			if err := checkURL(req.URL); err != nil {
				return fmt.Errorf("redirected to a URL the atlas policy refuses: %w", err)
			}
			return nil
		},
	}}
}

// Fetch GETs rawURL and returns its body, refusing a response larger than
// limit and folding a non-200 status into an error that names both the URL and
// the status.
func (c *Client) Fetch(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", SafeText(rawURL), err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", SafeText(rawURL), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %s", SafeText(rawURL), resp.Status)
	}
	// Read one byte past the cap so an over-limit body is detected rather than
	// silently truncated.
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", SafeText(rawURL), err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response exceeds the %d-byte limit — refusing to install", SafeText(rawURL), limit)
	}
	return data, nil
}
