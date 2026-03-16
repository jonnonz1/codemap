package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	anthropicAPI     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	maxSourceBytes   = 8000 // truncate large files to keep prompt small
)

// AnthropicSummarizer calls the Anthropic Messages API to generate
// summary, when_to_use, and keywords for a source file.
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

// Summarize sends the source file to Claude and parses back structured fields.
func (a *AnthropicSummarizer) Summarize(path string, source []byte) (*SummaryResult, error) {
	src := string(source)
	if len(src) > maxSourceBytes {
		src = src[:maxSourceBytes] + "\n... (truncated)"
	}

	ext := filepath.Ext(path)
	lang := langFromExt(ext)

	fence := "```"
	prompt := fmt.Sprintf("Analyze this %s source file and return ONLY a JSON object with these fields:\n"+
		"- \"summary\": A one-sentence description of what this file does (max 100 chars)\n"+
		"- \"when_to_use\": A one-sentence description of when a developer would need to read or edit this file (max 100 chars)\n"+
		"- \"keywords\": An array of 3-6 lowercase keywords describing the file's purpose and domain\n\n"+
		"File: %s\n\n%s%s\n%s\n%s\n\n"+
		"Return ONLY valid JSON, no markdown fencing, no explanation.", lang, path, fence, lang, src, fence)

	body := anthropicRequest{
		Model:     a.Model,
		MaxTokens: 256,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", anthropicAPI, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return parseAnthropicResponse(respBody)
}

type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

type summaryJSON struct {
	Summary   string   `json:"summary"`
	WhenToUse string   `json:"when_to_use"`
	Keywords  []string `json:"keywords"`
}

func parseAnthropicResponse(body []byte) (*SummaryResult, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing API response: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty API response")
	}

	text := resp.Content[0].Text

	// Strip markdown code fences if the model included them.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result summaryJSON
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing summary JSON %q: %w", truncate(text, 100), err)
	}

	return &SummaryResult{
		Summary:   result.Summary,
		WhenToUse: result.WhenToUse,
		Keywords:  result.Keywords,
	}, nil
}

func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".rb":
		return "Ruby"
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".md":
		return "Markdown"
	case ".sql":
		return "SQL"
	default:
		return "source"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
