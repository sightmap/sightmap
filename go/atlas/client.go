package atlas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Fetch limits. A timeout bounds how long a response may take, not how large
// it may be: 30 seconds of a gigabit link is several gigabytes, and both the
// index and an archive are held in memory while they are parsed. Corpora are
// kilobyte-scale YAML, so these caps are generous by orders of magnitude and
// exist to make a hostile or broken mirror fail fast.
const (
	// MaxIndexBytes caps the atlas index; it lists every published entry.
	MaxIndexBytes = 8 << 20 // 8 MiB
	// MaxArchiveBytes caps a corpus archive as it arrives on the wire. The
	// decompressed size is capped separately by [MaxCorpusBytes]: a gzip bomb
	// is small on the wire and enormous on disk.
	MaxArchiveBytes = 8 << 20 // 8 MiB
	// MaxRedirects caps the redirect chain of a single fetch.
	MaxRedirects = 5
	// FetchTimeout bounds a single fetch, end to end.
	FetchTimeout = 30 * time.Second
)

// HTTPError is a non-200 response. Callers match it with [errors.As] to react
// to a particular status — a 404 from an archive URL means the atlas publishes
// no such slug, which deserves better wording than "HTTP 404 Not Found".
type HTTPError struct {
	URL        string
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %s", SafeText(e.URL), e.Status)
}

// Client fetches atlas content under a fixed policy: HTTPS only (plain HTTP
// for loopback), applied to the requested URL and re-applied to every redirect
// hop, a bounded redirect chain, a response size cap, and an overall timeout.
// The redirect check is the load-bearing part — without it a mirror or a
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
// limit and folding a non-200 status into an [HTTPError] that names both the
// URL and the status.
//
// The transport policy is applied to rawURL before the request is built, so a
// refused URL is never dialled. The policy has to hold on the first hop for the
// same reason [NewClient] re-applies it to every redirect.
func (c *Client) Fetch(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", SafeText(rawURL), err)
	}
	if err := checkURL(u); err != nil {
		return nil, err
	}
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
		return nil, &HTTPError{URL: rawURL, StatusCode: resp.StatusCode, Status: resp.Status}
	}
	// Read one byte past the cap so an over-limit body is detected rather than
	// silently truncated.
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", SafeText(rawURL), err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response exceeds the %d-byte limit", SafeText(rawURL), limit)
	}
	return data, nil
}

// checkURL enforces the transport policy: HTTPS everywhere, with a plain-HTTP
// exception for loopback hosts so tests and local mirrors work. It runs on the
// requested URL and again on every redirect hop.
func checkURL(u *url.URL) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return fmt.Errorf("refusing non-HTTPS URL %s (plain http is allowed only for localhost)", SafeText(u.String()))
}
