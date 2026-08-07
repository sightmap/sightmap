package atlas

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Fetch limits. The timeout bounds how long a response may take, not how large
// it may be, so the size cap is separate: 30 seconds of a fast link is several
// gigabytes, and both callers hold the whole body in memory.
const (
	maxFetchBytes = 8 << 20 // 8 MiB
	maxRedirects  = 5
	fetchTimeout  = 30 * time.Second

	// IndexTTL is how long a cached catalog is served without asking the
	// network again. The atlas changes on the scale of days.
	IndexTTL = 24 * time.Hour
)

// httpError is a non-200 response, kept typed so a 404 can become "no such
// entry" without matching on message text.
type httpError struct {
	URL        string
	StatusCode int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d %s", e.URL, e.StatusCode, http.StatusText(e.StatusCode))
}

// client applies the transport policy on every hop. Re-checking each redirect
// is the load-bearing part: without it a mirror answering "302 Location:
// http://…" downgrades a fetch to plaintext after the scheme was approved.
var client = &http.Client{
	Timeout: fetchTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		if err := checkURL(req.URL); err != nil {
			return fmt.Errorf("redirected to a URL the atlas policy refuses: %w", err)
		}
		return nil
	},
}

// fetch GETs rawURL and returns its body, refusing anything the transport
// policy rejects and any response over maxFetchBytes.
func fetch(ctx context.Context, rawURL string) ([]byte, error) {
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", SafeText(rawURL), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{URL: SafeText(rawURL), StatusCode: resp.StatusCode}
	}
	// Read one byte past the cap so an over-limit body is detected rather than
	// silently truncated.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", SafeText(rawURL), err)
	}
	if len(data) > maxFetchBytes {
		return nil, fmt.Errorf("GET %s: response exceeds the %d-byte limit — refusing to install", SafeText(rawURL), maxFetchBytes)
	}
	return data, nil
}

// checkURL enforces HTTPS, with a plain-HTTP exception for loopback so tests
// and local mirrors work.
func checkURL(u *url.URL) error {
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && isLoopback(u.Hostname()):
		return nil
	}
	return fmt.Errorf("refusing non-HTTPS URL %s (plain http is allowed only for localhost)", u)
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// IndexOptions configures [LoadIndex].
type IndexOptions struct {
	// URL is the catalog to read. Empty means [DefaultIndexURL].
	URL string
	// Refresh fetches even when a fresh cache is on disk.
	Refresh bool
	// CacheDir overrides where the catalog is cached. Empty means
	// ~/.sightmap/atlas.
	CacheDir string
	// Now overrides the clock, for tests.
	Now func() time.Time
}

// IndexResult is a loaded catalog and where it came from.
type IndexResult struct {
	Index     *Index
	Source    string    // the URL the catalog was read from
	FetchedAt time.Time // when those bytes were fetched
	FromCache bool      // whether this call served them from disk
}

// cacheEnvelope is what lands on disk: the fetched bytes verbatim, plus what it
// takes to know whether they are still usable.
type cacheEnvelope struct {
	Source    string          `json:"source"`
	FetchedAt time.Time       `json:"fetched_at"`
	Index     json.RawMessage `json:"index"`
}

// LoadIndex returns the atlas catalog, from the cache when it is fresh and from
// the network otherwise.
//
// The cache is an optimization: a missing, corrupt, stale, or
// differently-sourced file is a miss rather than an error, and a cache that
// cannot be written costs a round trip rather than a search.
func LoadIndex(ctx context.Context, opts IndexOptions) (*IndexResult, error) {
	indexURL := cmp.Or(opts.URL, DefaultIndexURL)
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	path := cachePath(opts.CacheDir)

	if !opts.Refresh && path != "" {
		if env, ok := readCache(path); ok && env.Source == indexURL && now().Sub(env.FetchedAt) < IndexTTL {
			if idx, err := ParseIndex(env.Index); err == nil {
				return &IndexResult{Index: idx, Source: indexURL, FetchedAt: env.FetchedAt, FromCache: true}, nil
			}
		}
	}

	data, err := fetch(ctx, indexURL)
	if err != nil {
		return nil, fmt.Errorf("fetch atlas index: %w", err)
	}
	idx, err := ParseIndex(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SafeText(indexURL), err)
	}
	fetched := now()
	writeCache(path, cacheEnvelope{Source: indexURL, FetchedAt: fetched, Index: data})
	return &IndexResult{Index: idx, Source: indexURL, FetchedAt: fetched}, nil
}

// cachePath resolves the cache file, or "" when there is no usable home
// directory to put one in.
func cachePath(dir string) string {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".sightmap", "atlas")
	}
	return filepath.Join(dir, "index.json")
}

func readCache(path string) (cacheEnvelope, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEnvelope{}, false
	}
	var env cacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil || len(env.Index) == 0 {
		return cacheEnvelope{}, false
	}
	return env, true
}

// writeCache stores the catalog, ignoring every failure. A torn or unwritable
// cache file is a slower search on the next run, not a broken one, because
// [readCache] treats anything it cannot parse as a miss.
func writeCache(path string, env cacheEnvelope) {
	data, err := json.Marshal(env)
	if path == "" || err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		os.WriteFile(path, data, 0o644)
	}
}
