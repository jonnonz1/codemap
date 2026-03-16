package llm

import "testing"

func TestParseAnthropicResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		summary string
		wantErr bool
	}{
		{
			name: "clean JSON",
			body: `{"content":[{"text":"{\"summary\":\"Handles user auth\",\"when_to_use\":\"When modifying login flow\",\"keywords\":[\"auth\",\"login\",\"session\"]}"}]}`,
			summary: "Handles user auth",
		},
		{
			name: "JSON with markdown fences",
			body: `{"content":[{"text":"` + "```json\\n" + `{\"summary\":\"DB helpers\",\"when_to_use\":\"When querying\",\"keywords\":[\"db\"]}` + "\\n```" + `"}]}`,
			summary: "DB helpers",
		},
		{
			name:    "empty content",
			body:    `{"content":[]}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON in text",
			body:    `{"content":[{"text":"not json at all"}]}`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAnthropicResponse([]byte(tc.body))
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
