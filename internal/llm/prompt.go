package llm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const maxSourceBytes = 8000

// buildPrompt creates the summarization prompt for any provider.
func buildPrompt(path string, source []byte) string {
	src := string(source)
	if len(src) > maxSourceBytes {
		src = src[:maxSourceBytes] + "\n... (truncated)"
	}

	ext := filepath.Ext(path)
	lang := langFromExt(ext)
	fence := "```"

	return fmt.Sprintf("Analyze this %s source file and return ONLY a JSON object with these fields:\n"+
		"- \"summary\": A one-sentence description of what this file does (max 100 chars)\n"+
		"- \"when_to_use\": A one-sentence description of when a developer would need to read or edit this file (max 100 chars)\n"+
		"- \"keywords\": An array of 3-6 lowercase keywords describing the file's purpose and domain\n\n"+
		"File: %s\n\n%s%s\n%s\n%s\n\n"+
		"Return ONLY valid JSON, no markdown fencing, no explanation.", lang, path, fence, lang, src, fence)
}

// parseSummaryJSON extracts a SummaryResult from raw LLM text output.
func parseSummaryJSON(text string) (*SummaryResult, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result struct {
		Summary   string   `json:"summary"`
		WhenToUse string   `json:"when_to_use"`
		Keywords  []string `json:"keywords"`
	}
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
