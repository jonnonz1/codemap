package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicAPI     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// AnthropicSummarizer calls the Anthropic Messages API.
type AnthropicSummarizer struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewAnthropicSummarizer creates a summarizer that calls the Anthropic API.
// Model defaults to claude-haiku-4-5-20251001 if empty.
func NewAnthropicSummarizer(apiKey, model string) *AnthropicSummarizer {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &AnthropicSummarizer{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *AnthropicSummarizer) Summarize(path string, source []byte) (*SummaryResult, error) {
	prompt := buildPrompt(path, source)

	body, _ := json.Marshal(struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:     a.Model,
		MaxTokens: 256,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})

	req, err := http.NewRequest("POST", anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing anthropic response: %w", err)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty anthropic response")
	}

	return parseSummaryJSON(result.Content[0].Text)
}
