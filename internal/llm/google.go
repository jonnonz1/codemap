package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const googleAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"

// GoogleSummarizer calls the Google Gemini API.
type GoogleSummarizer struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewGoogleSummarizer creates a summarizer that calls the Gemini API.
// Model defaults to gemini-2.0-flash if empty.
func NewGoogleSummarizer(apiKey, model string) *GoogleSummarizer {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GoogleSummarizer{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GoogleSummarizer) Summarize(path string, source []byte) (*SummaryResult, error) {
	prompt := buildPrompt(path, source)

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", googleAPIBase, g.Model, g.APIKey)

	body, _ := json.Marshal(struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{{Parts: []struct {
			Text string `json:"text"`
		}{{Text: prompt}}}},
		GenerationConfig: struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		}{MaxOutputTokens: 256},
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing google response: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty google response")
	}

	return parseSummaryJSON(result.Candidates[0].Content.Parts[0].Text)
}
