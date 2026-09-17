package grow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// component is the YAML shape written for a grown component.
type component struct {
	Name        string     `yaml:"name"`
	Selector    string     `yaml:"selector"`
	Description string     `yaml:"description,omitempty"`
	Properties  []property `yaml:"properties,omitempty"`
}

type property struct {
	Name    string `yaml:"name"`
	Extract string `yaml:"extract"`
}

// writeComponent appends comp to the named view's components list (or to the
// global components.yaml when viewName is empty), then reloads the corpus and
// rolls the file back if it no longer validates.
func (g *Grower) writeComponent(viewName string, comp component) error {
	file, doc, err := g.locateView(viewName)
	if err != nil {
		return err
	}
	var seq *yaml.Node
	if viewName == "" {
		seq = ensureSequence(rootMapping(doc), "components")
	} else {
		view := findView(doc, viewName)
		if view == nil {
			return fmt.Errorf("view %s not found in %s", viewName, file)
		}
		seq = ensureSequence(view, "components")
	}
	node := &yaml.Node{}
	if err := node.Encode(comp); err != nil {
		return err
	}
	seq.Content = append(seq.Content, node)
	return g.commit(file, doc)
}

// promoteToGlobal moves a view-scoped component into components.yaml so it
// matches on every view.
func (g *Grower) promoteToGlobal(o owner) error {
	file, doc, err := g.locateView(o.view)
	if err != nil {
		return err
	}
	view := findView(doc, o.view)
	if view == nil {
		return fmt.Errorf("view %s not found", o.view)
	}
	seq := mappingValue(view, "components")
	if seq == nil {
		return fmt.Errorf("view %s has no components", o.view)
	}
	idx := -1
	for i, c := range seq.Content {
		if v := mappingValue(c, "name"); v != nil && v.Value == o.name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("component %s not found in view %s", o.name, o.view)
	}
	comp := seq.Content[idx]
	seq.Content = append(seq.Content[:idx], seq.Content[idx+1:]...)
	if d := mappingValue(comp, "description"); d != nil {
		d.Value += " (promoted to global: seen on several views)"
	}

	gfile := filepath.Join(g.Dir, "components.yaml")
	gdoc, err := loadDoc(gfile)
	if err != nil {
		return err
	}
	root := rootMapping(gdoc)
	if mappingValue(root, "version") == nil {
		root.Content = append([]*yaml.Node{{Kind: yaml.ScalarNode, Value: "version"}, {Kind: yaml.ScalarNode, Value: "1"}}, root.Content...)
	}
	gseq := ensureSequence(root, "components")
	for _, c := range gseq.Content {
		if v := mappingValue(c, "name"); v != nil && v.Value == o.name {
			return nil // already global
		}
	}
	gseq.Content = append(gseq.Content, comp)

	beforeView, _ := os.ReadFile(file)
	beforeGlobal, gErr := os.ReadFile(gfile)
	if err := writeDoc(file, doc); err != nil {
		return err
	}
	if err := writeDoc(gfile, gdoc); err != nil {
		os.WriteFile(file, beforeView, 0o644)
		return err
	}
	if err := g.reload(); err != nil {
		os.WriteFile(file, beforeView, 0o644)
		if gErr != nil {
			os.Remove(gfile)
		} else {
			os.WriteFile(gfile, beforeGlobal, 0o644)
		}
		return fmt.Errorf("promote %s: %w", o.name, err)
	}
	g.promoted++
	return nil
}

// commit writes doc to file, reloads, and restores the previous bytes on failure.
func (g *Grower) commit(file string, doc *yaml.Node) error {
	before, readErr := os.ReadFile(file)
	if err := writeDoc(file, doc); err != nil {
		return err
	}
	if err := g.reload(); err != nil {
		if readErr != nil {
			os.Remove(file)
		} else {
			os.WriteFile(file, before, 0o644)
		}
		return err
	}
	return nil
}

// locateView finds the YAML file that declares viewName. An empty name means
// the global components.yaml.
func (g *Grower) locateView(viewName string) (string, *yaml.Node, error) {
	if viewName == "" {
		file := filepath.Join(g.Dir, "components.yaml")
		doc, err := loadDoc(file)
		return file, doc, err
	}
	var files []string
	filepath.Walk(g.Dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == "snapshots" || strings.HasPrefix(info.Name(), ".") && p != g.Dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml") {
			files = append(files, p)
		}
		return nil
	})
	for _, f := range files {
		doc, err := loadDoc(f)
		if err != nil {
			continue
		}
		if findView(doc, viewName) != nil {
			return f, doc, nil
		}
	}
	return "", nil, fmt.Errorf("no YAML file declares view %s", viewName)
}

func loadDoc(file string) (*yaml.Node, error) {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	return &doc, nil
}

func writeDoc(file string, doc *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	enc.Close()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func rootMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensureSequence returns the sequence at key in mapping m, creating or
// replacing a null/flow-empty value with a block sequence.
func ensureSequence(m *yaml.Node, key string) *yaml.Node {
	v := mappingValue(m, key)
	if v == nil {
		v = &yaml.Node{Kind: yaml.SequenceNode}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
		return v
	}
	if v.Kind != yaml.SequenceNode {
		v.Kind = yaml.SequenceNode
		v.Tag = ""
		v.Value = ""
		v.Content = nil
	}
	v.Style = 0 // block style, so an inline [] becomes a list on write
	return v
}

// findView returns the mapping node of the view named name in doc, or nil.
func findView(doc *yaml.Node, name string) *yaml.Node {
	views := mappingValue(rootMapping(doc), "views")
	if views == nil || views.Kind != yaml.SequenceNode {
		return nil
	}
	for _, v := range views.Content {
		if n := mappingValue(v, "name"); n != nil && n.Value == name {
			return v
		}
	}
	return nil
}
