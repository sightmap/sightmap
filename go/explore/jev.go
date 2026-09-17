package explore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// JevDefaultBaseURL is the TypeSafe API that serves Jev.
const JevDefaultBaseURL = "https://api.typesafe.ai"

// JevPicker asks Jev (via the TypeSafe "system one" endpoint) typed questions:
// a choice over the option keys and a yes/no on "done". Wire format:
//
//	POST {base}/v1/systemone
//	{ "state": {...}, "model": "jev-latest", "questions": { name: {type, criteria, instructions} } }
//
// Answers come back as {choice, confidence, probabilities} for a choice and
// {noul} for a yes/no.
type JevPicker struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
	stats   Stats
}

// NewJevPicker reads TYPESAFE_API_KEY (and TYPESAFE_BASE_URL) from the environment.
func NewJevPicker(model string) (*JevPicker, error) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("TYPESAFE_API_KEY is not set (the jev picker needs it)")
	}
	base := os.Getenv("TYPESAFE_BASE_URL")
	if base == "" {
		base = JevDefaultBaseURL
	}
	if model == "" {
		model = "jev-latest"
	}
	return &JevPicker{APIKey: key, BaseURL: base, Model: model, Client: &http.Client{Timeout: 25 * time.Second}}, nil
}

func (j *JevPicker) Name() string { return "jev:" + j.Model }

func (j *JevPicker) Stats() Stats { return j.stats }

type jevQuestion struct {
	Type         string            `json:"type"`
	Criteria     map[string]string `json:"criteria,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
}

type jevAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          float64            `json:"noul"`
}

type jevResponse struct {
	Model   string               `json:"model"`
	Usage   map[string]int       `json:"usage"`
	Answers map[string]jevAnswer `json:"answers"`
}

func choiceQuestion(crit Criteria, instructions string) jevQuestion {
	m := make(map[string]string, len(crit.Options))
	for _, o := range crit.Options {
		m[o.Key] = o.Desc
	}
	return jevQuestion{Type: "choice", Criteria: m, Instructions: instructions}
}

func noulQuestion(instructions string) jevQuestion {
	return jevQuestion{Type: "noul", Instructions: instructions}
}

// ask posts one state with a set of questions. It retries on 429 and 5xx.
func (j *JevPicker) ask(ctx context.Context, state string, questions map[string]jevQuestion) (*jevResponse, int, error) {
	body, err := json.Marshal(map[string]interface{}{
		"state":     map[string]string{"context": state},
		"model":     j.Model,
		"questions": questions,
	})
	if err != nil {
		return nil, 0, err
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		t0 := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.BaseURL+"/v1/systemone", bytes.NewReader(body))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Authorization", "Bearer "+j.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-TypeSafe-SDK", "sightmap-explore")
		res, err := j.Client.Do(req)
		if err != nil {
			lastErr = err
			sleepCtx(ctx, time.Duration(300*(attempt+1))*time.Millisecond)
			continue
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		ms := int(time.Since(t0).Milliseconds())
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			lastErr = fmt.Errorf("jev http %d: %s", res.StatusCode, truncBytes(data, 200))
			wait := time.Duration(400*(attempt+1)) * time.Millisecond
			if ra := res.Header.Get("retry-after-ms"); ra != "" {
				if n, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(n) * time.Millisecond
				}
			} else if ra := res.Header.Get("retry-after"); ra != "" {
				if n, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(n) * time.Second
				}
			}
			sleepCtx(ctx, wait)
			continue
		}
		if res.StatusCode != 200 {
			return nil, ms, fmt.Errorf("jev http %d: %s", res.StatusCode, truncBytes(data, 300))
		}
		var out jevResponse
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, ms, fmt.Errorf("jev: decode response: %w", err)
		}
		j.stats.Calls++
		j.stats.Ms += ms
		j.stats.InputTokens += out.Usage["input_tokens"]
		j.stats.OutputTokens += out.Usage["output_tokens"]
		return &out, ms, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("jev: exhausted retries")
	}
	return nil, 0, lastErr
}

const pickInstructions = "Pick the single best next action to move toward the GOAL. Prefer the element that directly advances the goal (a group entry means \"look inside that group\"). If a required value must be typed first, pick that field."
const doneInstructions = "The GOAL is already fully achieved on the current page as described. Answer yes only if nothing else is needed."

func (j *JevPicker) Pick(ctx context.Context, state string, crit Criteria) (Pick, error) {
	res, ms, err := j.ask(ctx, state, map[string]jevQuestion{
		"next": choiceQuestion(crit, pickInstructions),
		"done": noulQuestion(doneInstructions),
	})
	if err != nil {
		return Pick{}, err
	}
	next := res.Answers["next"]
	return Pick{
		Next:  fallbackKey(crit, next.Choice),
		Done:  res.Answers["done"].Noul,
		Probs: next.Probabilities,
		Ms:    ms,
	}, nil
}

func (j *JevPicker) Choose(ctx context.Context, state string, crit Criteria, instructions string) (string, error) {
	res, _, err := j.ask(ctx, state, map[string]jevQuestion{"pick": choiceQuestion(crit, instructions)})
	if err != nil {
		return "", err
	}
	return fallbackKey(crit, res.Answers["pick"].Choice), nil
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func truncBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
