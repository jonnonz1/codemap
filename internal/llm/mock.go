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

// SummarizeBatch returns mock summaries for each file in the batch.
func (m *MockSummarizer) SummarizeBatch(paths []string, _ [][]byte) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(paths))
	for i, path := range paths {
		base := filepath.Base(path)
		results[i] = &SummaryResult{
			Summary:   "Source file " + base,
			WhenToUse: "When working with " + base,
			Keywords:  []string{filepath.Ext(path)},
		}
	}
	return results, nil
}
