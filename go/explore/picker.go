package explore

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Pick is a picker's answer for one step.
type Pick struct {
	Next  string             // the chosen option key
	Done  float64            // 0..1 confidence that the goal is already met
	Probs map[string]float64 // per-option probabilities when the picker reports them
	Why   string             // free-text reason when the picker gives one
	Ms    int                // model round-trip time
}

// Stats is a picker's running usage.
type Stats struct {
	Calls        int     `json:"calls"`
	Ms           int     `json:"ms"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	USD          float64 `json:"usd"` // 0 when the picker has no known price
}

// Picker answers the loop's two typed questions.
type Picker interface {
	// Name identifies the picker in output, e.g. "jev:jev-latest".
	Name() string
	// Pick chooses one option key and judges whether the goal is already met.
	Pick(ctx context.Context, state string, crit Criteria) (Pick, error)
	// Choose answers a one-off multiple-choice question (which value, which option).
	Choose(ctx context.Context, state string, crit Criteria, instructions string) (string, error)
	// Stats reports usage so far.
	Stats() Stats
}

// topProbs renders the k most likely options as "key:0.93 ...".
func topProbs(p map[string]float64, k int) string {
	type kv struct {
		k string
		v float64
	}
	var all []kv
	for key, v := range p {
		all = append(all, kv{key, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	var parts []string
	for i, e := range all {
		if i >= k {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%.2f", e.k, e.v))
	}
	return strings.Join(parts, " ")
}

// fallbackKey returns key when it is an option, else the first option.
func fallbackKey(crit Criteria, key string) string {
	if crit.Has(key) {
		return key
	}
	if len(crit.Options) > 0 {
		return crit.Options[0].Key
	}
	return ""
}
