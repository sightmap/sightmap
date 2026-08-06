package atlas

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IndexTTL is how long a cached index is served without asking the network
// again. The atlas changes on the scale of days; a search should not spend a
// round trip on every invocation.
const IndexTTL = 24 * time.Hour

// CacheDir returns the directory sightmap caches the atlas index in
// (~/.sightmap/atlas), alongside the browser cache at ~/.sightmap/browsers.
func CacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sightmap", "atlas"), nil
}

// IndexOptions configures [LoadIndex].
type IndexOptions struct {
	// URL is the index to read. Empty means [DefaultIndexURL].
	URL string
	// Refresh fetches even when a fresh cache is on disk.
	Refresh bool
	// TTL overrides [IndexTTL]. Zero means [IndexTTL].
	TTL time.Duration
	// CacheDir overrides [CacheDir]. Empty means [CacheDir]; "-" disables the
	// cache entirely.
	CacheDir string
	// Client fetches the index. Nil means [NewClient].
	Client *Client
	// Now overrides the clock, for tests.
	Now func() time.Time
}

// IndexResult is a loaded index and where it came from.
type IndexResult struct {
	Index     *Index
	Source    string    // the URL the index was read from
	FetchedAt time.Time // when those bytes were fetched
	FromCache bool      // whether this call served them from disk
}

// Age reports how old the index bytes are.
func (r *IndexResult) Age(now time.Time) time.Duration {
	if r.FetchedAt.IsZero() {
		return 0
	}
	return now.Sub(r.FetchedAt)
}

// cacheEnvelope is what lands in ~/.sightmap/atlas/index.json: the fetched
// index verbatim, plus what it takes to know whether it is still usable —
// which URL it came from, and when.
type cacheEnvelope struct {
	Source    string          `json:"source"`
	FetchedAt time.Time       `json:"fetched_at"`
	Index     json.RawMessage `json:"index"`
}

// LoadIndex returns the atlas index, from the cache when it is fresh and from
// the network otherwise.
//
// The cache is an optimization and is treated as one: a missing, corrupt,
// stale, or differently-sourced cache file is a miss, never an error. Failing
// to write it is not an error either, so a read-only HOME costs a round trip
// rather than a search.
func LoadIndex(ctx context.Context, opts IndexOptions) (*IndexResult, error) {
	indexURL := opts.URL
	if indexURL == "" {
		indexURL = DefaultIndexURL
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = IndexTTL
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	path := cachePath(opts.CacheDir)
	if !opts.Refresh && path != "" {
		if env, ok := readCache(path); ok && env.Source == indexURL && now().Sub(env.FetchedAt) < ttl {
			if idx, err := ParseIndex(env.Index); err == nil {
				return &IndexResult{Index: idx, Source: indexURL, FetchedAt: env.FetchedAt, FromCache: true}, nil
			}
		}
	}

	client := opts.Client
	if client == nil {
		client = NewClient()
	}
	data, err := client.Fetch(ctx, indexURL, MaxIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch atlas index: %w", err)
	}
	idx, err := ParseIndex(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SafeText(indexURL), err)
	}
	fetched := now()
	if path != "" {
		writeCache(path, cacheEnvelope{Source: indexURL, FetchedAt: fetched, Index: data})
	}
	return &IndexResult{Index: idx, Source: indexURL, FetchedAt: fetched}, nil
}

// cachePath resolves the cache file, or "" when there is no usable cache.
func cachePath(dir string) string {
	if dir == "-" {
		return ""
	}
	if dir == "" {
		var err error
		if dir, err = CacheDir(); err != nil {
			return ""
		}
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

// writeCache replaces the cache file atomically. Every failure is ignored: a
// cache that cannot be written is a slower search, not a broken one.
func writeCache(path string, env cacheEnvelope) {
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "index-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
	}
}
