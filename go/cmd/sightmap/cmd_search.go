package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Local minimal structs — do not import internal sightmap loader types.
type srchFile struct {
	Version    int        `yaml:"version"`
	Components []srchComp `yaml:"components"`
	Views      []srchView `yaml:"views"`
	Requests   []srchReq  `yaml:"requests"`
}

type srchReq struct {
	Name string `yaml:"name"`
}

type srchView struct {
	Name       string     `yaml:"name"`
	Route      string     `yaml:"route"`
	Components []srchComp `yaml:"components"`
}

type srchComp struct {
	Name        string     `yaml:"name"`
	Ref         string     `yaml:"$ref"`
	Selector    string     `yaml:"selector"`
	Description string     `yaml:"description"`
	Memory      []string   `yaml:"memory"`
	Children    []srchComp `yaml:"children"`
}

func runSearch(args []string) error {
	fs2 := flag.NewFlagSet("search", flag.ContinueOnError)
	sightmapDir := fs2.String("sightmap-dir", ".sightmap", "Path to .sightmap/ directory")
	field := fs2.String("field", "all", "Field to search: name, selector, description, memory, all")
	caseSensitive := fs2.Bool("case", false, "Enable case-sensitive matching")

	if err := fs2.Parse(args); err != nil {
		return err
	}

	remaining := fs2.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("PATTERN is required\nUsage: sightmap search [--sightmap-dir DIR] [--field FIELD] [--case] PATTERN")
	}
	pattern := remaining[0]

	// Validate --field value.
	switch *field {
	case "name", "selector", "description", "memory", "all":
		// ok
	default:
		return fmt.Errorf("--field must be one of: name, selector, description, memory, all (got %q)", *field)
	}

	// Compile regexp.
	rePattern := pattern
	if !*caseSensitive {
		rePattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	matchCount := 0
	// Name hits on views/requests — search covers component fields only, so these
	// don't count as matches, but they explain a "no matches" near-miss.
	var viewHits, reqHits []string
	nameSearch := *field == "all" || *field == "name"

	walkErr := filepath.WalkDir(*sightmapDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories (other than the root sightmapDir itself).
			if strings.HasPrefix(d.Name(), ".") && path != *sightmapDir {
				return fs.SkipDir
			}
			return nil
		}
		// Skip hidden files.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		// Relative path for display.
		relPath, relErr := filepath.Rel(*sightmapDir, path)
		if relErr != nil {
			relPath = path
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "warning: read %s: %v\n", relPath, readErr)
			return nil
		}

		var sf srchFile
		if unmarshalErr := yaml.Unmarshal(data, &sf); unmarshalErr != nil {
			fmt.Fprintf(os.Stderr, "warning: parse %s: %v\n", relPath, unmarshalErr)
			return nil
		}

		// Walk top-level components.
		for _, comp := range sf.Components {
			n := searchComp(comp, re, *field, []string{relPath}, relPath, "", "")
			matchCount += n
		}

		// Walk views.
		for _, view := range sf.Views {
			if nameSearch && view.Name != "" && re.MatchString(view.Name) {
				viewHits = append(viewHits, view.Name)
			}
			viewCrumb := fmt.Sprintf("%s [%s %s]", relPath, view.Name, view.Route)
			for _, comp := range view.Components {
				n := searchComp(comp, re, *field, []string{viewCrumb}, relPath, view.Name, view.Route)
				matchCount += n
			}
		}
		for _, r := range sf.Requests {
			if nameSearch && r.Name != "" && re.MatchString(r.Name) {
				reqHits = append(reqHits, r.Name)
			}
		}

		return nil
	})

	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", *sightmapDir, walkErr)
	}

	if matchCount == 0 {
		var notes []string
		if len(viewHits) > 0 {
			notes = append(notes, fmt.Sprintf("a view named %s", strings.Join(viewHits, ", ")))
		}
		if len(reqHits) > 0 {
			notes = append(notes, fmt.Sprintf("a request named %s", strings.Join(reqHits, ", ")))
		}
		if len(notes) > 0 {
			fmt.Fprintf(os.Stderr, "no component matches for %q — but the corpus has %s. "+
				"search covers component fields (name, selector, description, memory), not view or request names.\n",
				pattern, strings.Join(notes, " and "))
		} else {
			fmt.Fprintf(os.Stderr, "no matches for %s\n", pattern)
		}
	}

	return nil
}

// searchComp recursively walks a component tree, printing matches.
// breadcrumbs holds the display path up to (but not including) this component.
// Returns the number of matches printed.
func searchComp(comp srchComp, re *regexp.Regexp, field string, breadcrumbs []string, relPath, viewName, viewRoute string) int {
	count := 0

	isRef := comp.Ref != "" && comp.Name == ""

	// Build this component's own crumb label.
	var crumbLabel string
	if isRef {
		crumbLabel = "$ref: " + comp.Ref
	} else {
		crumbLabel = comp.Name
	}

	// Determine which fields matched.
	type matchedField struct {
		key   string
		value string
	}
	var matched []matchedField

	switch field {
	case "name":
		if isRef {
			if re.MatchString(comp.Ref) {
				matched = append(matched, matchedField{"$ref", comp.Ref})
			}
		} else {
			if re.MatchString(comp.Name) {
				matched = append(matched, matchedField{"name", comp.Name})
			}
		}
	case "selector":
		if re.MatchString(comp.Selector) && comp.Selector != "" {
			matched = append(matched, matchedField{"selector", comp.Selector})
		}
	case "description":
		if re.MatchString(comp.Description) && comp.Description != "" {
			matched = append(matched, matchedField{"description", comp.Description})
		}
	case "memory":
		if len(comp.Memory) > 0 {
			joined := strings.Join(comp.Memory, "\n")
			if re.MatchString(joined) {
				matched = append(matched, matchedField{"memory", joined})
			}
		}
	case "all":
		if isRef {
			if re.MatchString(comp.Ref) {
				matched = append(matched, matchedField{"$ref", comp.Ref})
			}
		} else {
			if re.MatchString(comp.Name) {
				matched = append(matched, matchedField{"name", comp.Name})
			}
		}
		if comp.Selector != "" && re.MatchString(comp.Selector) {
			matched = append(matched, matchedField{"selector", comp.Selector})
		}
		if comp.Description != "" && re.MatchString(comp.Description) {
			matched = append(matched, matchedField{"description", comp.Description})
		}
		if len(comp.Memory) > 0 {
			joined := strings.Join(comp.Memory, "\n")
			if re.MatchString(joined) {
				matched = append(matched, matchedField{"memory", joined})
			}
		}
	}

	if len(matched) > 0 {
		// Print the breadcrumb line.
		crumbs := append(breadcrumbs, crumbLabel) //nolint:gocritic
		fmt.Println(strings.Join(crumbs, " › "))

		// Print matched fields (skip "name" — it's already in the breadcrumb).
		for _, mf := range matched {
			if mf.key == "name" {
				continue
			}
			// For memory: indent each line.
			if mf.key == "memory" {
				lines := strings.Split(mf.value, "\n")
				fmt.Printf("  memory: %s\n", lines[0])
				for _, l := range lines[1:] {
					fmt.Printf("    %s\n", l)
				}
			} else {
				fmt.Printf("  %s: %s\n", mf.key, mf.value)
			}
		}
		fmt.Println()
		count++
	}

	// Recurse into children.
	myBreadcrumbs := append(breadcrumbs, crumbLabel) //nolint:gocritic
	for _, child := range comp.Children {
		count += searchComp(child, re, field, myBreadcrumbs, relPath, viewName, viewRoute)
	}

	return count
}
