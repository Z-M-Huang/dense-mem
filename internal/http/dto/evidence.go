package dto

// Evidence represents first-class provenance returned on claim and fact reads.
type Evidence struct {
	FragmentID        string  `json:"fragment_id"`
	Speaker           string  `json:"speaker,omitempty"`
	SpanStart         int     `json:"span_start"`
	SpanEnd           int     `json:"span_end"`
	ExtractConf       float64 `json:"extract_conf"`
	ExtractionModel   string  `json:"extraction_model,omitempty"`
	ExtractionVersion string  `json:"extraction_version,omitempty"`
	PipelineRunID     string  `json:"pipeline_run_id,omitempty"`
	Authority         string  `json:"authority,omitempty"`
}
