package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

// surveyPattern is one entry from survey.yaml.
type surveyPattern struct {
	pattern string
	reason  string
}

func runDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	addrFlag := fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	launchFlag := fs.Bool("launch", false, "Auto-launch Chrome if unreachable")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	allFlag := fs.Bool("all", false, "Include surveyed (○) patterns in output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	// ── Connect to Chrome ────────────────────────────────────────────────────
	cleanup := func() {}
	defer func() { cleanup() }()

	conn, dialErr := dialAddrTab(*addrFlag, *tabFlag)
	if dialErr != nil {
		if !*launchFlag {
			return dialErr
		}
		var launchCleanup func()
		var launchErr error
		conn, launchCleanup, launchErr = browser.Launch(ctx, browser.LaunchOptions{})
		if launchErr != nil {
			return fmt.Errorf("discover: cannot launch Chrome: %v", launchErr)
		}
		if launchCleanup != nil {
			cleanup = launchCleanup
		}
	} else {
		cleanup = func() { conn.Close() }
	}

	// ── Extract internal links from the current page ─────────────────────────
	const linkScript = `(function() {
  const host = location.host;
  const seen = new Set();
  const links = [];
  for (const a of document.querySelectorAll('a[href]')) {
    try {
      const u = new URL(a.href);
      if (u.host === host && !seen.has(u.pathname)) {
        seen.add(u.pathname);
        links.push(u.pathname);
      }
    } catch(e) {}
  }
  return links;
})()`

	raw, err := browser.EvalJSON(ctx, conn, linkScript)
	if err != nil {
		return fmt.Errorf("discover: eval links: %v", err)
	}

	var pathnames []string
	if err := json.Unmarshal(raw, &pathnames); err != nil {
		return fmt.Errorf("discover: parse links: %v", err)
	}

	// ── Normalise paths → patterns; count how many raw paths map to each ──────
	patternCounts := make(map[string]int)
	for _, p := range pathnames {
		patternCounts[normalisePath(p)]++
	}

	// ── Load corpus (optional — absent dir = treat all patterns as unseen) ────
	var corpus *sightmap.Corpus
	if _, statErr := os.Stat(*sightmapDirFlag); statErr == nil {
		loader := sightmap.DirLoader(*sightmapDirFlag)
		corpus, err = loader.Load()
		if err != nil {
			return fmt.Errorf("discover: load corpus: %v", err)
		}
	}

	// ── Load survey.yaml (optional) ───────────────────────────────────────────
	// surveyPatterns holds ordered (pattern, reason) pairs for glob matching.
	var surveyPatterns []surveyPattern
	surveyPath := filepath.Join(*sightmapDirFlag, "survey.yaml")
	if data, readErr := os.ReadFile(surveyPath); readErr == nil {
		var surveyFile struct {
			Surveyed []struct {
				Pattern string `yaml:"pattern"`
				Reason  string `yaml:"reason"`
			} `yaml:"surveyed"`
		}
		if parseErr := yaml.Unmarshal(data, &surveyFile); parseErr == nil {
			for _, e := range surveyFile.Surveyed {
				if e.Pattern != "" {
					surveyPatterns = append(surveyPatterns, surveyPattern{
						pattern: e.Pattern,
						reason:  e.Reason,
					})
				}
			}
		}
	}

	// ── Classify each pattern ─────────────────────────────────────────────────
	type discoverEntry struct {
		pattern string
		status  string // "?", "✓", "○"
		label   string
		count   int // raw-path count for "?" entries
	}

	var unseen, mapped, surveyed []discoverEntry

	for pattern, count := range patternCounts {
		// ✓ mapped — corpus has a view whose route matches this pattern
		if corpus != nil {
			if v := corpus.ViewForURL(pattern); v != nil {
				mapped = append(mapped, discoverEntry{
					pattern: pattern,
					status:  "✓",
					label:   fmt.Sprintf("[%s]", v.Name),
					count:   count,
				})
				continue
			}
		}

		// ○ surveyed — matches any survey.yaml pattern (glob)
		if sp, ok := matchSurvey(surveyPatterns, pattern); ok {
			label := "(surveyed)"
			if sp.reason != "" {
				label = fmt.Sprintf("(surveyed: %s)", sp.reason)
			}
			surveyed = append(surveyed, discoverEntry{
				pattern: pattern,
				status:  "○",
				label:   label,
				count:   count,
			})
			continue
		}

		// ? unseen
		noun := "links"
		if count == 1 {
			noun = "link"
		}
		unseen = append(unseen, discoverEntry{
			pattern: pattern,
			status:  "?",
			label:   fmt.Sprintf("(unseen \u2014 %d %s)", count, noun),
			count:   count,
		})
	}

	// ── Sort each group ───────────────────────────────────────────────────────
	// Unseen: by count desc, then pattern asc (highest-traffic gaps first)
	sort.Slice(unseen, func(i, j int) bool {
		if unseen[i].count != unseen[j].count {
			return unseen[i].count > unseen[j].count
		}
		return unseen[i].pattern < unseen[j].pattern
	})
	// Mapped: alphabetical by pattern
	sort.Slice(mapped, func(i, j int) bool {
		return mapped[i].pattern < mapped[j].pattern
	})
	// Surveyed: alphabetical by pattern
	sort.Slice(surveyed, func(i, j int) bool {
		return surveyed[i].pattern < surveyed[j].pattern
	})

	// ── Assemble output order: unseen → mapped → surveyed (if --all) ─────────
	var rows []discoverEntry
	rows = append(rows, unseen...)
	rows = append(rows, mapped...)
	if *allFlag {
		rows = append(rows, surveyed...)
	}

	if len(rows) == 0 {
		fmt.Println("No patterns discovered.")
		return nil
	}

	for _, e := range rows {
		fmt.Printf("%-3s %-40s %s\n", e.status, e.pattern, e.label)
	}

	return nil
}

// normalisePath replaces dynamic path segments with "*".
// A segment is dynamic if it is all digits, a UUID, or a slug-like token
// (8+ lowercase alphanumeric/dash chars) that contains at least one digit.
func normalisePath(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range segments {
		if isDynamicSegment(seg) {
			segments[i] = "*"
		}
	}
	return "/" + strings.Join(segments, "/")
}

var (
	reAllDigits = regexp.MustCompile(`^\d+$`)
	reSlugDyn   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,}$`) // 8+ chars
	reUUID      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reHasDigit  = regexp.MustCompile(`\d`)
)

func isDynamicSegment(seg string) bool {
	if seg == "" {
		return false
	}
	lower := strings.ToLower(seg)
	return reAllDigits.MatchString(seg) ||
		reUUID.MatchString(lower) ||
		(reSlugDyn.MatchString(lower) && reHasDigit.MatchString(lower))
}

// matchSurvey returns the first survey pattern that glob-matches discoveredPattern,
// using the same ** / * semantics as sightmap view route matching.
func matchSurvey(patterns []surveyPattern, discoveredPattern string) (surveyPattern, bool) {
	for _, sp := range patterns {
		re := routePatternToRegex(sp.pattern)
		if compiled, err := regexp.Compile(re); err == nil {
			if compiled.MatchString(discoveredPattern) {
				return sp, true
			}
		}
	}
	return surveyPattern{}, false
}

// routePatternToRegex converts a route glob (with ** and *) to an anchored regexp.
// Mirrors sightmap/corpus.go:routeToRegex but defined locally to avoid importing internals.
func routePatternToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteByte('^')
	for i := 0; i < len(pattern); {
		if pattern[i] == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString("(.+)")
				i += 2
				continue
			}
			sb.WriteString("([^/]+)")
			i++
			continue
		}
		c := pattern[i]
		switch c {
		case '.', '+', '?', '(', ')', '[', ']', '{', '}', '\\', '|', '^', '$':
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
		i++
	}
	sb.WriteByte('$')
	return sb.String()
}
