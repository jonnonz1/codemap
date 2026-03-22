// Package llm defines the summarizer interface for LLM-based enrichment.
package llm

// SummaryResult holds the semantic fields produced by an LLM summarizer.
type SummaryResult struct {
	Summary   string
	WhenToUse string
	Keywords  []string
}

// Summarizer enriches a source file with semantic metadata.
// Implementations may call a real LLM or return mock values.
type Summarizer interface {
	Summarize(path string, source []byte) (*SummaryResult, error)
}

// BatchSummarizer extends Summarizer with multi-file batch support.
// Implementations pack multiple files into a single API call to reduce
// round-trip overhead and per-request token costs.
type BatchSummarizer interface {
	Summarizer
	SummarizeBatch(paths []string, sources [][]byte) ([]*SummaryResult, error)
}
