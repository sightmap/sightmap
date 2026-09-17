package explore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// AnthropicClient calls the Messages API. It serves two roles: the optional
// once-per-goal planner (goal text -> Spec) and a baseline Picker that answers
// the same questions a big model in the same seat would.
type AnthropicClient struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
	// Price per million tokens, used only for the reported USD estimate.
	InputUSDPerM  float64
	OutputUSDPerM float64
	stats         Stats
}

// NewAnthropicClient reads ANTHROPIC_API_KEY from the environment.
func NewAnthropicClient(model string) (*AnthropicClient, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}
	if model == "" {
		model = "claude-sonnet-5"
	}
	return &AnthropicClient{
		APIKey: key, Model: model, BaseURL: "https://api.anthropic.com",
		Client:        &http.Client{Timeout: 90 * time.Second},
		InputUSDPerM:  3,
		OutputUSDPerM: 15,
	}, nil
}

func (a *AnthropicClient) Name() string { return "anthropic:" + a.Model }

func (a *AnthropicClient) Stats() Stats {
	s := a.stats
	s.USD = (float64(s.InputTokens)*a.InputUSDPerM + float64(s.OutputTokens)*a.OutputUSDPerM) / 1e6
	return s
}

type anthropicResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// message sends one user turn and returns the text content. maxTokens must
// leave room for a thinking block: models that think before answering return
// an empty text when the cap is too small.
func (a *AnthropicClient) message(ctx context.Context, system, user string, maxTokens int) (string, int, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":      a.Model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	})
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		t0 := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			return "", 0, err
		}
		req.Header.Set("x-api-key", a.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")
		res, err := a.Client.Do(req)
		if err != nil {
			lastErr = err
			sleepCtx(ctx, time.Duration(800*(attempt+1))*time.Millisecond)
			continue
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		ms := int(time.Since(t0).Milliseconds())
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			lastErr = fmt.Errorf("anthropic http %d: %s", res.StatusCode, truncBytes(data, 200))
			sleepCtx(ctx, time.Duration(800*(attempt+1))*time.Millisecond)
			continue
		}
		if res.StatusCode != 200 {
			return "", ms, fmt.Errorf("anthropic http %d: %s", res.StatusCode, truncBytes(data, 300))
		}
		var out anthropicResponse
		if err := json.Unmarshal(data, &out); err != nil {
			return "", ms, fmt.Errorf("anthropic: decode: %w", err)
		}
		a.stats.Calls++
		a.stats.Ms += ms
		a.stats.InputTokens += out.Usage.InputTokens
		a.stats.OutputTokens += out.Usage.OutputTokens
		var text strings.Builder
		for _, c := range out.Content {
			if c.Type == "text" {
				text.WriteString(c.Text)
			}
		}
		return text.String(), ms, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("anthropic: exhausted retries")
	}
	return "", 0, lastErr
}

var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// jsonReply asks for a JSON object and decodes it into v, retrying once with a nudge.
func (a *AnthropicClient) jsonReply(ctx context.Context, system, user string, maxTokens int, v interface{}) (int, error) {
	text, ms, err := a.message(ctx, system, user, maxTokens)
	if err != nil {
		return ms, err
	}
	m := jsonObjectRe.FindString(text)
	if m == "" {
		text2, ms2, err := a.message(ctx, system, user+"\n\nReply with the JSON object only.", maxTokens)
		ms += ms2
		if err != nil {
			return ms, err
		}
		m = jsonObjectRe.FindString(text2)
		if m == "" {
			return ms, fmt.Errorf("anthropic: no JSON in reply: %q", trunc(text2, 200))
		}
	}
	if err := json.Unmarshal([]byte(m), v); err != nil {
		return ms, fmt.Errorf("anthropic: bad JSON in reply: %w", err)
	}
	return ms, nil
}

const pickerSystem = `You are the step picker for a browser agent. You see the goal, recent steps, and the actionable elements on the current page, each with a key.
Reply with JSON only: {"next": "<key>", "done": true|false, "why": "<ten words>"}.
"done" is true only if the goal is already fully achieved on the current page.
Pick exactly one key from the ACTIONS list (a key starting with "g:" means "look inside that group of elements"). Do not invent keys.`

func (a *AnthropicClient) Pick(ctx context.Context, state string, crit Criteria) (Pick, error) {
	var lines []string
	for _, o := range crit.Options {
		lines = append(lines, o.Key+": "+o.Desc)
	}
	user := state + "\n\nACTIONS (key: description):\n" + strings.Join(lines, "\n")
	var reply struct {
		Next string `json:"next"`
		Done bool   `json:"done"`
		Why  string `json:"why"`
	}
	ms, err := a.jsonReply(ctx, pickerSystem, user, 1024, &reply)
	if err != nil {
		return Pick{}, err
	}
	next := fallbackKey(crit, strings.TrimSpace(reply.Next))
	done := 0.0
	if reply.Done {
		done = 1
	}
	return Pick{Next: next, Done: done, Probs: map[string]float64{next: 1}, Why: reply.Why, Ms: ms}, nil
}

func (a *AnthropicClient) Choose(ctx context.Context, state string, crit Criteria, instructions string) (string, error) {
	var lines []string
	for _, o := range crit.Options {
		lines = append(lines, o.Key+": "+o.Desc)
	}
	var reply struct {
		Pick string `json:"pick"`
	}
	_, err := a.jsonReply(ctx, `Reply with JSON only: {"pick": "<key>"}`, state+"\n\n"+instructions+"\nOPTIONS:\n"+strings.Join(lines, "\n"), 512, &reply)
	if err != nil {
		return "", err
	}
	return fallbackKey(crit, strings.TrimSpace(reply.Pick)), nil
}

const plannerSystem = `You turn a browsing goal into a small JSON spec for a step-by-step browser agent.
The agent can click, fill text fields, pick select options, go back, and scroll. It cannot invent text: every value it types must come from "values".
Reply with JSON only:
{
  "done_when": { "url_contains"?: string, "view"?: string, "text_contains"?: string, "component"?: string, "prop"?: {"component": string, "name": string, "contains": string} },
  "values": { "<short_key>": "<literal text to type>" },
  "hint": "<one sentence of advice for choosing steps, or empty>"
}
Use only the done_when fields you are confident about. Prefer url_contains or text_contains. Keys in values are lowercase snake_case that describe the field.`

// Plan asks the model to write the Spec for a goal.
func (a *AnthropicClient) Plan(ctx context.Context, goal, site string) (*Spec, int, error) {
	var spec Spec
	ms, err := a.jsonReply(ctx, plannerSystem, fmt.Sprintf("Site: %s\nGoal: %s", site, goal), 1024, &spec)
	if err != nil {
		return nil, ms, err
	}
	if spec.Values == nil {
		spec.Values = map[string]string{}
	}
	return &spec, ms, nil
}
