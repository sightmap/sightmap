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
	Memory     []string       `yaml:"memory"`
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
	URL         string         `yaml:"url"`
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
	Source      string         `yaml:"source"`
	Memory      []string       `yaml:"memory"`
	Tags        []string       `yaml:"tags"`
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

	var memory []string
	var globalRaws []rawComponent
	type viewFileWithPath struct {
		rf   rawFile
		path string
	}
	var viewFiles []viewFileWithPath
	var fieldDiags []ValidationError

	for _, p := range yamlPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("sightmap: read %q: %w", p, err)
		}
		var rf rawFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("sightmap: parse %q: %w", p, err)
		}
		// Tooling files (config.yaml, survey.yaml) have their own schemas and are
		// not corpus files — don't run the corpus unknown-field check over them.
		if base := filepath.Base(p); base != "config.yaml" && base != "config.yml" && base != "survey.yaml" && base != "survey.yml" {
			fieldDiags = append(fieldDiags, unknownFieldWarnings(data, base)...)
		}
		memory = append(memory, rf.Memory...)
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

	// Warn on duplicate top-level global component names. This must run on the
	// RAW globals, not the flattened list: flattening a child component reused
	// under several parents yields multiple same-name entries, which would
	// otherwise look like a collision.
	ctx.diagnostics = append(ctx.diagnostics, globalNameCollisions(globalRaws)...)

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
			// Per-view url wins; fall back to the file-level url as a default.
			viewURL := rv.URL
			if viewURL == "" {
				viewURL = vf.URL
			}
			views = append(views, View{
				Name:       rv.Name,
				Route:      rv.Route,
				Memory:     rv.Memory,
				Components: flattenAll(rv.Components, ctx),
				Stability:  rv.Stability,
				Access:     access,
				URL:        viewURL,
				Snapshots:  snapshots,
				SourceFile: basename,
			})
		}
	}

	return &Corpus{
		Memory:           memory,
		GlobalComponents: globalComps,
		Views:            views,
		loadDiagnostics:  append(ctx.diagnostics, fieldDiags...),
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

// globalNameCollisions warns when two or more top-level global components share a
// name with different selectors. A duplicated global name is ambiguous — both
// match every view and resolution falls back to declaration order. (Same
// name + same selector is a true duplicate, reported as an error elsewhere, so
// it is skipped here.)
func globalNameCollisions(globals []rawComponent) []ValidationError {
	type acc struct {
		count int
		sels  map[string]bool
	}
	byName := map[string]*acc{}
	var order []string
	for _, g := range globals {
		if g.Name == "" {
			continue
		}
		a := byName[g.Name]
		if a == nil {
			a = &acc{sels: map[string]bool{}}
			byName[g.Name] = a
			order = append(order, g.Name)
		}
		a.count++
		a.sels[string(g.Selector)] = true
	}
	var out []ValidationError
	for _, name := range order {
		a := byName[name]
		if a.count < 2 || len(a.sels) < 2 {
			continue
		}
		out = append(out, ValidationError{
			Component: name,
			Code:      "merge-collision-component",
			Severity:  SeverityWarning,
			Message: fmt.Sprintf("global component name %q is defined %d times with different selectors; only the first applies to a given node",
				name, a.count),
		})
	}
	return out
}

// flattenCtx carries the shared state for a flattening pass: the $ref registry
// and any structural diagnostics discovered along the way (currently circular
// $ref chains, which are expanded away and so invisible downstream).
type flattenCtx struct {
	reg         map[string]rawComponent
	diagnostics []ValidationError
	seen        map[string]bool // dedupe key (code + identity) → already-reported
}

// addDiag appends a diagnostic once per distinct key, so a component reused via
// $ref at several sites does not report the same problem repeatedly.
func (ctx *flattenCtx) addDiag(key string, ve ValidationError) {
	if ctx.seen == nil {
		ctx.seen = map[string]bool{}
	}
	if ctx.seen[key] {
		return
	}
	ctx.seen[key] = true
	ctx.diagnostics = append(ctx.diagnostics, ve)
}

// recordCircular records a $ref cycle diagnostic once per distinct chain.
func (ctx *flattenCtx) recordCircular(chain []string) {
	ctx.addDiag("ref-circular\x00"+strings.Join(chain, "\x00"), ValidationError{
		Component: chain[len(chain)-1],
		Code:      "ref-circular",
		Severity:  SeverityError,
		Message:   "circular $ref chain " + strings.Join(chain, " → "),
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
			// Unresolved $ref: the referenced global does not exist. The spec
			// requires this to be a hard error, so surface it instead of
			// silently dropping the reference.
			ctx.addDiag("ref-unresolved\x00"+rc.Ref, ValidationError{
				Component: rc.Ref,
				Code:      "ref-unresolved",
				Severity:  SeverityError,
				Message:   fmt.Sprintf("$ref %q does not resolve to any global component", rc.Ref),
			})
			return nil
		}
		refStack = append(refStack, rc.Ref)
		rc = global
	}

	// A real (non-$ref) component that lacks a required field would otherwise be
	// dropped silently, so it never reaches Validate. Surface it as an error
	// (schema requires both name and selector).
	if rc.Name == "" {
		ctx.addDiag("missing-name\x00"+string(rc.Selector), ValidationError{
			Selector: string(rc.Selector),
			Code:     "missing-name",
			Severity: SeverityError,
			Message:  "component is missing a name",
		})
		return nil
	}
	if string(rc.Selector) == "" {
		ctx.addDiag("missing-selector\x00"+rc.Name, ValidationError{
			Component: rc.Name,
			Code:      "missing-selector",
			Severity:  SeverityError,
			Message:   "component is missing a selector",
		})
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
		Source:      rc.Source,
		Memory:      rc.Memory,
		Tags:        rc.Tags,
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
