package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicCaller makes raw LLM calls to the Anthropic API.
// Implements autoctx.LLMCaller.
type AnthropicCaller struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewAnthropicCaller(apiKey, model string) *AnthropicCaller {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &AnthropicCaller{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *AnthropicCaller) Call(prompt string) (string, error) {
	body, _ := json.Marshal(struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:     c.Model,
		MaxTokens: 1024,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
	})

	req, _ := http.NewRequest("POST", anthropicAPI, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Content[0].Text, nil
}

// OpenAICaller makes raw LLM calls to the OpenAI API.
type OpenAICaller struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewOpenAICaller(apiKey, model string) *OpenAICaller {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAICaller{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *OpenAICaller) Call(prompt string) (string, error) {
	body, _ := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}{
		Model: c.Model,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}},
		MaxTokens: 1024,
	})

	req, _ := http.NewRequest("POST", openaiAPI, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}

// GoogleCaller makes raw LLM calls to the Gemini API.
type GoogleCaller struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewGoogleCaller(apiKey, model string) *GoogleCaller {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GoogleCaller{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *GoogleCaller) Call(prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", googleAPIBase, c.Model, c.APIKey)

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
		}{MaxOutputTokens: 1024},
	})

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("google API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("google API %d: %s", resp.StatusCode, truncate(string(respBody), 200))
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
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// MockCaller returns all candidate files from the prompt for testing.
// It parses the code map section to extract file paths, so select works
// without a real LLM.
type MockCaller struct{}

func (c *MockCaller) Call(prompt string) (string, error) {
	// Extract file paths from the code map in the prompt.
	// Lines matching "- path/to/file.ext" are candidate files.
	var files []string
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			path := strings.TrimPrefix(line, "- ")
			// Only include lines that look like file paths (contain a dot extension).
			if strings.Contains(path, ".") && !strings.Contains(path, " ") {
				files = append(files, path)
			}
		}
	}

	filesJSON, _ := json.Marshal(files)
	return fmt.Sprintf(`{"context_files": %s, "reasoning": "mock caller — selected all candidates"}`, filesJSON), nil
}
