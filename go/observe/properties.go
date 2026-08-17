package observe

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

//go:embed properties.js
var propertiesJS string

// ExtractProperties runs a single batched JS evaluation against the live DOM to
// extract property values for all matched nodes that have property definitions,
// folding the results into each ComponentMatch's Properties (in definition
// order). At most 200 nodes are evaluated to avoid JS timeout.
func ExtractProperties(
	ctx context.Context,
	conn *browser.CDPConn,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	compByName map[string]sightmap.ComponentDef,
) {
	type specProp struct {
		Name      string `json:"name"`
		Extract   string `json:"extract"`
		Transform string `json:"transform"`
	}
	type spec struct {
		ID       string     `json:"id"`
		Selector string     `json:"selector"`
		Props    []specProp `json:"props"`
	}

	var specs []spec
	const maxNodes = 200
	for node, m := range matches {
		if len(specs) >= maxNodes {
			break
		}
		comp, ok := compByName[m.Name]
		if !ok || len(comp.Properties) == 0 || len(comp.Selectors) == 0 {
			continue
		}
		sp := spec{
			ID:       node.Id,
			Selector: comp.Selectors[0],
			Props:    make([]specProp, len(comp.Properties)),
		}
		for i, p := range comp.Properties {
			sp.Props[i] = specProp{Name: p.Name, Extract: p.Extract, Transform: p.Transform}
		}
		specs = append(specs, sp)
	}
	if len(specs) == 0 {
		return
	}

	specsJSON, err := json.Marshal(specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "observe: marshal property specs: %v\n", err)
		return
	}

	script := browser.DeepQueryJS + propertiesJS + fmt.Sprintf("\n__smExtractProperties(%s)", string(specsJSON))

	resultJSON, evalErr := browser.EvalJSON(ctx, conn, script)
	if evalErr != nil {
		fmt.Fprintf(os.Stderr, "observe: property extraction: %v\n", evalErr)
		return
	}

	var propValues map[string]map[string]string
	if err := json.Unmarshal(resultJSON, &propValues); err != nil {
		fmt.Fprintf(os.Stderr, "observe: property extraction unmarshal: %v\n", err)
		return
	}

	// Fold the extracted values into each match, in the definition's property
	// order, keeping only values that were actually extracted.
	for node, m := range matches {
		vals := propValues[node.Id]
		if len(vals) == 0 {
			continue
		}
		comp, ok := compByName[m.Name]
		if !ok {
			continue
		}
		var props []sightmap.PropertyValue
		for _, p := range comp.Properties {
			if v, ok := vals[p.Name]; ok {
				props = append(props, sightmap.PropertyValue{Name: p.Name, Value: v})
			}
		}
		m.Properties = props
	}
}
