package webmcp

// Corpus loader — a direct port of webmcp/src/corpus.js. Reads a .sightmap/
// directory per the spec (recursive *.yaml/*.yml discovery, shallow-append
// merge, $ref expansion against the root-level registry) and builds the
// breadcrumb-indexed component/request/view tables the compiler resolves
// manifest references against.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type componentEntry struct {
	Path        string
	Name        string
	ChainLevels [][]string // each level: alternative selectors, first match wins
	PropOrder   []string
	Props       map[string]propSpec
	Scope       string // "" = global, else owning view name
}

type propSpec struct {
	Extract   string
	Transform string
}

type requestEntry struct {
	Name        string
	Route       string
	Method      string
	Description string
	Properties  []any // raw ordered property mappings (*OM)
}

type viewEntry struct {
	Name        string
	Route       string
	URL         string
	Description string
}

type Corpus struct {
	Dir               string
	Files             []string
	Components        map[string][]*componentEntry
	crumbSeq          []string            // breadcrumbs in first-indexed order
	ByName            map[string][]string // component name -> breadcrumbs, first-seen order
	Views             map[string]*viewEntry
	ViewOrder         []string
	Requests          map[string]*requestEntry
	ReqOrder          []string
	DuplicateRequests map[string]bool
}

func listYamlFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip the reference CLI's non-corpus subdirectories, as its own
		// loader does: review/ holds punch-list YAML, snapshots/ holds
		// snapshot blobs.
		if d.IsDir() && p != dir && (d.Name() == "review" || d.Name() == "snapshots") {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// buildRegistry collects root-level components across files, first-seen wins.
func buildRegistry(docs []any) map[string]*OM {
	registry := map[string]*OM{}
	for _, doc := range docs {
		for _, c := range asList(omGet(doc, "components")) {
			comp := asOM(c)
			if comp == nil {
				continue
			}
			name := asString(omGet(comp, "name"))
			if name != "" && !comp.Has("$ref") {
				if _, ok := registry[name]; !ok {
					registry[name] = comp
				}
			}
		}
	}
	return registry
}

func expandRefs(list []any, registry map[string]*OM, seen map[string]bool) ([]*OM, error) {
	var out []*OM
	for _, entry := range list {
		om := asOM(entry)
		if om == nil {
			continue
		}
		if refV, ok := om.Get("$ref"); ok {
			ref := asString(refV)
			target, found := registry[ref]
			if !found {
				return nil, fmt.Errorf("ref-unresolved: no root-level component named %q", ref)
			}
			if seen[ref] {
				return nil, fmt.Errorf("ref-circular: %q expands through itself", ref)
			}
			next := map[string]bool{}
			for k := range seen {
				next[k] = true
			}
			next[ref] = true
			expanded, err := expandComponent(target, registry, next)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded)
		} else if om.Has("name") {
			expanded, err := expandComponent(om, registry, seen)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded)
		}
	}
	return out, nil
}

func expandComponent(comp *OM, registry map[string]*OM, seen map[string]bool) (*OM, error) {
	copy := NewOM()
	for _, k := range comp.Keys() {
		v, _ := comp.Get(k)
		copy.Set(k, v)
	}
	children, err := expandRefs(asList(omGet(comp, "children")), registry, seen)
	if err != nil {
		return nil, err
	}
	childAny := make([]any, len(children))
	for i, c := range children {
		childAny[i] = c
	}
	copy.Set("children", childAny)
	return copy, nil
}

func selectorAlternatives(selector any) []string {
	if l, ok := selector.([]any); ok {
		out := make([]string, len(l))
		for i, s := range l {
			out[i] = asString(s)
		}
		return out
	}
	return []string{asString(selector)}
}

func (c *Corpus) indexComponent(comp *OM, prefixPath string, prefixLevels [][]string, scope string) {
	name := asString(omGet(comp, "name"))
	crumb := name
	if prefixPath != "" {
		crumb = prefixPath + " " + name
	}
	levels := make([][]string, len(prefixLevels), len(prefixLevels)+1)
	copy(levels, prefixLevels)
	levels = append(levels, selectorAlternatives(omGet(comp, "selector")))

	entry := &componentEntry{
		Path:        crumb,
		Name:        name,
		ChainLevels: levels,
		Props:       map[string]propSpec{},
		Scope:       scope,
	}
	for _, p := range asList(omGet(comp, "properties")) {
		pom := asOM(p)
		if pom == nil {
			continue
		}
		pname := asString(omGet(pom, "name"))
		extract := asString(omGet(pom, "extract"))
		if pname == "" || extract == "" {
			continue
		}
		if _, dup := entry.Props[pname]; !dup {
			entry.PropOrder = append(entry.PropOrder, pname)
		}
		entry.Props[pname] = propSpec{Extract: extract, Transform: asString(omGet(pom, "transform"))}
	}
	if _, ok := c.Components[crumb]; !ok {
		c.crumbSeq = append(c.crumbSeq, crumb)
	}
	c.Components[crumb] = append(c.Components[crumb], entry)

	for _, child := range asList(omGet(comp, "children")) {
		if com := asOM(child); com != nil {
			c.indexComponent(com, crumb, levels, scope)
		}
	}
}

func sameChain(a, b *componentEntry) bool {
	if len(a.ChainLevels) != len(b.ChainLevels) {
		return false
	}
	for i := range a.ChainLevels {
		if len(a.ChainLevels[i]) != len(b.ChainLevels[i]) {
			return false
		}
		for j := range a.ChainLevels[i] {
			if a.ChainLevels[i][j] != b.ChainLevels[i][j] {
				return false
			}
		}
	}
	return true
}

func (c *Corpus) addRequest(r any) {
	rom := asOM(r)
	if rom == nil {
		return
	}
	name := asString(omGet(rom, "name"))
	if name == "" {
		return
	}
	if _, ok := c.Requests[name]; ok {
		c.DuplicateRequests[name] = true
		return
	}
	c.Requests[name] = &requestEntry{
		Name:        name,
		Route:       asString(omGet(rom, "route")),
		Method:      asString(omGet(rom, "method")),
		Description: asString(omGet(rom, "description")),
		Properties:  asList(omGet(rom, "properties")),
	}
	c.ReqOrder = append(c.ReqOrder, name)
}

// LoadCorpus loads and indexes a .sightmap/ directory.
func LoadCorpus(dir string) (*Corpus, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("sightmap corpus not found: %s is not a directory", dir)
	}
	files, err := listYamlFiles(dir)
	if err != nil {
		return nil, err
	}
	var docs []any
	var loadedFiles []string
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		doc, err := parseYAMLOrdered(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p, err)
		}
		if doc == nil {
			continue
		}
		// A YAML file without "version: 1" is a tooling file (survey.yaml
		// and friends), not a corpus file — skip it, as the reference
		// loader does.
		if v, _ := asInt(omGet(doc, "version")); v != 1 {
			continue
		}
		docs = append(docs, doc)
		loadedFiles = append(loadedFiles, p)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("sightmap corpus at %s contains no corpus YAML files (version: 1)", dir)
	}
	files = loadedFiles

	registry := buildRegistry(docs)
	c := &Corpus{
		Dir:               dir,
		Files:             files,
		Components:        map[string][]*componentEntry{},
		ByName:            map[string][]string{},
		Views:             map[string]*viewEntry{},
		Requests:          map[string]*requestEntry{},
		DuplicateRequests: map[string]bool{},
	}

	for _, doc := range docs {
		rootComps, err := expandRefs(asList(omGet(doc, "components")), registry, map[string]bool{})
		if err != nil {
			return nil, err
		}
		for _, comp := range rootComps {
			c.indexComponent(comp, "", nil, "")
		}
		for _, r := range asList(omGet(doc, "requests")) {
			c.addRequest(r)
		}
		for _, v := range asList(omGet(doc, "views")) {
			vom := asOM(v)
			if vom == nil {
				continue
			}
			vname := asString(omGet(vom, "name"))
			if vname == "" {
				continue
			}
			if _, ok := c.Views[vname]; !ok {
				url := asString(omGet(vom, "url"))
				if url == "" {
					url = asString(omGet(doc, "url"))
				}
				c.Views[vname] = &viewEntry{
					Name:        vname,
					Route:       asString(omGet(vom, "route")),
					URL:         url,
					Description: asString(omGet(vom, "description")),
				}
				c.ViewOrder = append(c.ViewOrder, vname)
			}
			viewComps, err := expandRefs(asList(omGet(vom, "components")), registry, map[string]bool{})
			if err != nil {
				return nil, err
			}
			for _, comp := range viewComps {
				c.indexComponent(comp, "", nil, vname)
			}
			for _, r := range asList(omGet(vom, "requests")) {
				c.addRequest(r)
			}
		}
	}

	// Collapse duplicate identical entries (a merged entry keeps the widest
	// scope), then build the name -> breadcrumbs index in first-indexed crumb
	// order. crumbSeq is that insertion order.
	for _, crumb := range c.crumbSeq {
		entries := c.Components[crumb]
		var unique []*componentEntry
		for _, e := range entries {
			var dup *componentEntry
			for _, u := range unique {
				if sameChain(u, e) {
					dup = u
					break
				}
			}
			if dup != nil {
				if dup.Scope != e.Scope {
					dup.Scope = ""
				}
			} else {
				unique = append(unique, e)
			}
		}
		c.Components[crumb] = unique
		name := crumb[strings.LastIndex(crumb, " ")+1:]
		c.ByName[name] = append(c.ByName[name], crumb)
	}
	return c, nil
}
