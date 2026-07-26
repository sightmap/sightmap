package sightmap

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sightmap/sightmap/go/match"
)

// Loader is the source of sightmap corpus data.
type Loader interface {
	Load() (*Corpus, error)
}

// LoaderFunc adapts any func() (*Corpus, error) to the Loader interface.
type LoaderFunc func() (*Corpus, error)

// Load implements Loader.
func (f LoaderFunc) Load() (*Corpus, error) { return f() }

// StaticLoader returns a Loader backed by an already-constructed Corpus.
func StaticLoader(c *Corpus) Loader {
	return LoaderFunc(func() (*Corpus, error) { return c, nil })
}

// DirLoader reads all YAML files from a .sightmap/ directory tree.
// Files with a top-level components: key are treated as global component
// files; files with a top-level views: key are treated as view files.
func DirLoader(path string) Loader {
	return LoaderFunc(func() (*Corpus, error) {
		return loadDir(path)
	})
}

// ---- raw YAML types (unexported) --------------------------------------------

type rawFile struct {
	Version    int            `yaml:"version"`
	Components []rawComponent `yaml:"components"`
	Views      []rawView      `yaml:"views"`
	URL        string         `yaml:"url"`
	Snapshots  []rawSnapshot  `yaml:"snapshots"`
}

type rawSnapshot struct {
	Name  string `yaml:"name"`
	Notes string `yaml:"notes"`
	URL   string `yaml:"url"`
}

type rawView struct {
	Name        string         `yaml:"name"`
	Route       string         `yaml:"route"`
	Description string         `yaml:"description"`
	Memory      []string       `yaml:"memory"`
	Components  []rawComponent `yaml:"components"`
	Stability   string         `yaml:"stability"`
	Access      *rawAccess     `yaml:"access"`
}

type rawAccess struct {
	Status string `yaml:"status"`
	Reason string `yaml:"reason"`
}

type rawProperty struct {
	Name      string `yaml:"name"`
	Extract   string `yaml:"extract"`
	Transform string `yaml:"transform"`
}

type rawComponent struct {
	Name        string         `yaml:"name"`
	Ref         string         `yaml:"$ref"`
	Selector    rawSelector    `yaml:"selector"`
	Description string         `yaml:"description"`
	Memory      []string       `yaml:"memory"`
	Children    []rawComponent `yaml:"children"`
	Properties  []rawProperty  `yaml:"properties"`
	Stability   string         `yaml:"stability"`
}

// rawSelector handles the selector field as either a scalar string or a YAML
// sequence of strings, joining them with a comma so splitSelectors can handle both.
type rawSelector string

func (r *rawSelector) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*r = rawSelector(value.Value)
		return nil
	case yaml.SequenceNode:
		var parts []string
		if err := value.Decode(&parts); err != nil {
			return err
		}
		*r = rawSelector(strings.Join(parts, ","))
		return nil
	default:
		return fmt.Errorf("unexpected YAML node kind %v for selector", value.Kind)
	}
}

// ---- DirLoader implementation -----------------------------------------------

func loadDir(path string) (*Corpus, error) {
	// Collect all .yaml / .yml files in lexical order.
	var yamlPaths []string
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip non-corpus subdirectories: review/ holds punch-list YAML
			// sequences (not corpus files), snapshots/ holds snapshot blobs.
			if p != path && (d.Name() == "review" || d.Name() == "snapshots") {
				return fs.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".yaml" || ext == ".yml" {
			yamlPaths = append(yamlPaths, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sightmap: walk %q: %w", path, err)
	}

	var globalRaws []rawComponent
	type viewFileWithPath struct {
		rf   rawFile
		path string
	}
	var viewFiles []viewFileWithPath

	for _, p := range yamlPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("sightmap: read %q: %w", p, err)
		}
		var rf rawFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("sightmap: parse %q: %w", p, err)
		}
		if len(rf.Components) > 0 {
			globalRaws = append(globalRaws, rf.Components...)
		}
		if len(rf.Views) > 0 {
			viewFiles = append(viewFiles, viewFileWithPath{rf: rf, path: p})
		}
	}

	// Build the global registry used for $ref resolution.
	reg := make(map[string]rawComponent, len(globalRaws))
	for _, gc := range globalRaws {
		if gc.Name != "" {
			reg[gc.Name] = gc
		}
	}

	// One flatten context is shared across the global list and every view so
	// that $ref cycle detection and diagnostics accumulate in one place.
	ctx := &flattenCtx{reg: reg}

	// Flatten global components (hierarchy → compound descendant selectors).
	globalComps := flattenAll(globalRaws, ctx)

	// Build views, expanding $refs and flattening each view's component list.
	var views []View
	for _, vfp := range viewFiles {
		vf := vfp.rf
		// Extract source file basename (without extension)
		basename := filepath.Base(vfp.path)
		basename = strings.TrimSuffix(basename, filepath.Ext(basename))

		// Convert snapshots
		var snapshots []Snapshot
		for _, rs := range vf.Snapshots {
			snapshots = append(snapshots, Snapshot{
				Name:  rs.Name,
				Notes: rs.Notes,
				URL:   rs.URL,
			})
		}

		for _, rv := range vf.Views {
			var access Access
			if rv.Access != nil {
				access = Access{Status: rv.Access.Status, Reason: rv.Access.Reason}
			}
			views = append(views, View{
				Name:       rv.Name,
				Route:      rv.Route,
				Memory:     rv.Memory,
				Components: flattenAll(rv.Components, ctx),
				Stability:  rv.Stability,
				Access:     access,
				URL:        vf.URL,
				Snapshots:  snapshots,
				SourceFile: basename,
			})
		}
	}

	return &Corpus{
		GlobalComponents: globalComps,
		Views:            views,
		loadDiagnostics:  ctx.diagnostics,
	}, nil
}

// ---- flattening helpers -----------------------------------------------------

// rawPropsToMatch converts a slice of rawProperty to match.Property.
func rawPropsToMatch(rps []rawProperty) []match.Property {
	if len(rps) == 0 {
		return nil
	}
	ps := make([]match.Property, len(rps))
	for i, rp := range rps {
		ps[i] = match.Property{Name: rp.Name, Extract: rp.Extract, Transform: rp.Transform}
	}
	return ps
}

// flattenCtx carries the shared state for a flattening pass: the $ref registry
// and any structural diagnostics discovered along the way (currently circular
// $ref chains, which are expanded away and so invisible downstream).
type flattenCtx struct {
	reg          map[string]rawComponent
	diagnostics  []ValidationError
	seenCircular map[string]bool // dedupe key → already-reported
}

// recordCircular records a $ref cycle diagnostic once per distinct chain.
func (ctx *flattenCtx) recordCircular(chain []string) {
	key := strings.Join(chain, "\x00")
	if ctx.seenCircular == nil {
		ctx.seenCircular = map[string]bool{}
	}
	if ctx.seenCircular[key] {
		return
	}
	ctx.seenCircular[key] = true
	ctx.diagnostics = append(ctx.diagnostics, ValidationError{
		Component: chain[len(chain)-1],
		Message:   "ref-circular: circular $ref chain " + strings.Join(chain, " → "),
	})
}

// flattenAll flattens a slice of rawComponents into a flat list of
// SightmapComponents with compound descendant selectors.
func flattenAll(rcs []rawComponent, ctx *flattenCtx) []match.SightmapComponent {
	var result []match.SightmapComponent
	for _, rc := range rcs {
		result = append(result, flattenOne(rc, nil, ctx, nil, nil)...)
	}
	return result
}

// flattenOne recursively flattens a single rawComponent.
// parentSels holds the already-computed selectors of the nearest ancestor;
// they are prepended (with a space) to every alternative in this component's
// selector. parentChain is the slice of ancestor component names (root-first)
// carried through recursion and stored on each SightmapComponent so the
// extension can scope child selectors to their parent's DOM subtree.
// refStack is the chain of $ref names currently being expanded; it guards
// against circular references, which would otherwise recurse forever.
func flattenOne(rc rawComponent, parentSels []string, ctx *flattenCtx, parentChain []string, refStack []string) []match.SightmapComponent {
	// Expand $ref: replace the placeholder with a deep copy of the named global.
	if rc.Ref != "" {
		for _, prev := range refStack {
			if prev == rc.Ref {
				// Cycle: stop expanding rather than recurse forever.
				ctx.recordCircular(append(append([]string(nil), refStack...), rc.Ref))
				return nil
			}
		}
		global, ok := ctx.reg[rc.Ref]
		if !ok {
			return nil // unknown ref — reported separately by strict validation
		}
		refStack = append(refStack, rc.Ref)
		rc = global
	}

	// Skip components that lack a name or a selector.
	if rc.Name == "" || string(rc.Selector) == "" {
		return nil
	}

	rawSels := splitSelectors(string(rc.Selector))
	if len(rawSels) == 0 {
		return nil
	}

	// Combine with parent selectors: for every parent × every child alternative,
	// produce "parent child" (descendant combinator).
	var mySels []string
	if len(parentSels) == 0 {
		mySels = rawSels
	} else {
		mySels = make([]string, 0, len(parentSels)*len(rawSels))
		for _, ps := range parentSels {
			for _, cs := range rawSels {
				mySels = append(mySels, ps+" "+cs)
			}
		}
	}

	result := []match.SightmapComponent{{
		Name:        rc.Name,
		Selectors:   mySels,
		Memory:      rc.Memory,
		Properties:  rawPropsToMatch(rc.Properties),
		ParentChain: parentChain, // nil for top-level; omitted from JSON
		Stability:   rc.Stability,
	}}

	// Recurse into children: extend the parent chain with this component's name.
	childChain := append(append([]string(nil), parentChain...), rc.Name)
	for _, child := range rc.Children {
		result = append(result, flattenOne(child, mySels, ctx, childChain, refStack)...)
	}

	return result
}

// splitSelectors splits a comma-separated selector string and trims whitespace
// from each alternative, returning only non-empty parts. The split ignores
// commas that are not list separators: those inside a bracket or paren group
// (`[attr="a,b"]`, `:is()`, `:where()`, `:not()`, `:nth-child()`, …) or inside a
// quoted string. A backslash escapes the following character.
func splitSelectors(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	depth, start := 0, 0
	var quote byte // 0 outside a quoted string, else the opening quote char
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			i++ // skip the escaped character
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				if p := strings.TrimSpace(s[start:i]); p != "" {
					parts = append(parts, p)
				}
				start = i + 1
			}
		}
	}
	if p := strings.TrimSpace(s[start:]); p != "" {
		parts = append(parts, p)
	}
	return parts
}
