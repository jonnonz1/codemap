package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openaiAPI = "https://api.openai.com/v1/chat/completions"

// OpenAISummarizer calls the OpenAI Chat Completions API.
type OpenAISummarizer struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewOpenAISummarizer creates a summarizer that calls the OpenAI API.
// Model defaults to gpt-4o-mini if empty.
func NewOpenAISummarizer(apiKey, model string) *OpenAISummarizer {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAISummarizer{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAISummarizer) Summarize(path string, source []byte) (*SummaryResult, error) {
	prompt := buildPrompt(path, source)

	body, _ := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}{
		Model: o.Model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
		MaxTokens: 256,
	})

	req, err := http.NewRequest("POST", openaiAPI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing openai response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty openai response")
	}

	return parseSummaryJSON(result.Choices[0].Message.Content)
}
