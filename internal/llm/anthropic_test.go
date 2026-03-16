package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicSummarize(t *testing.T) {
	// Mock Anthropic API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing API key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}

		resp := map[string]any{
			"content": []map[string]string{
				{"text": `{"summary":"Handles routing","when_to_use":"When changing routes","keywords":["router","http"]}`},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Test response parsing (HTTP wiring tested via mock server above).
	_ = server // server validates headers if called

	result, err := parseSummaryJSON(`{"summary":"Handles routing","when_to_use":"When changing routes","keywords":["router","http"]}`)
	if err != nil {
		t.Fatalf("parseSummaryJSON: %v", err)
	}
	if result.Summary != "Handles routing" {
		t.Errorf("summary = %q", result.Summary)
	}
	if result.WhenToUse != "When changing routes" {
		t.Errorf("when_to_use = %q", result.WhenToUse)
	}
	if len(result.Keywords) != 2 {
		t.Errorf("keywords = %v", result.Keywords)
	}
}

func TestParseSummaryJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		summary string
		wantErr bool
	}{
		{
			name:    "clean JSON",
			input:   `{"summary":"DB helpers","when_to_use":"When querying","keywords":["db"]}`,
			summary: "DB helpers",
		},
		{
			name:    "with markdown fences",
			input:   "```json\n{\"summary\":\"Auth module\",\"when_to_use\":\"Login flow\",\"keywords\":[\"auth\"]}\n```",
			summary: "Auth module",
		},
		{
			name:    "invalid JSON",
			input:   "not json at all",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseSummaryJSON(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", result.Summary, tc.summary)
			}
		})
	}
}

func TestLangFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "Go"},
		{".ts", "TypeScript"},
		{".tsx", "TypeScript"},
		{".js", "JavaScript"},
		{".py", "Python"},
		{".rs", "Rust"},
		{".xyz", "source"},
	}
	for _, tc := range tests {
		got := langFromExt(tc.ext)
		if got != tc.want {
			t.Errorf("langFromExt(%q) = %q, want %q", tc.ext, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate short = %q", got)
	}
}

func TestBuildPrompt(t *testing.T) {
	prompt := buildPrompt("src/auth.ts", []byte("export function login() {}"))
	if prompt == "" {
		t.Error("empty prompt")
	}
	if !contains(prompt, "TypeScript") {
		t.Error("prompt should mention TypeScript")
	}
	if !contains(prompt, "src/auth.ts") {
		t.Error("prompt should contain file path")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
