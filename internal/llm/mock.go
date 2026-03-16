package llm

import "path/filepath"

// MockSummarizer returns placeholder summary values without calling any LLM.
type MockSummarizer struct{}

// Summarize returns a generic summary derived from the file path.
func (m *MockSummarizer) Summarize(path string, _ []byte) (*SummaryResult, error) {
	base := filepath.Base(path)
	return &SummaryResult{
		Summary:   "Source file " + base,
		WhenToUse: "When working with " + base,
		Keywords:  []string{filepath.Ext(path)},
	}, nil
}
