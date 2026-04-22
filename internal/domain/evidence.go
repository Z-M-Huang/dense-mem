package domain

// Authority represents the trust tier of a fragment or evidence source.
type Authority string

const (
	AuthorityAuthoritative Authority = "authoritative"
	AuthorityPrimary       Authority = "primary"
	AuthoritySecondary     Authority = "secondary"
	AuthorityInferred      Authority = "inferred"
	AuthorityUnknown       Authority = "unknown"
)

// IsValid reports whether a is a recognised Authority value.
func (a Authority) IsValid() bool {
	switch a {
	case AuthorityAuthoritative, AuthorityPrimary, AuthoritySecondary, AuthorityInferred, AuthorityUnknown:
		return true
	}
	return false
}

// Evidence captures the provenance chain for a claim or fact as exposed to
// external callers.
type Evidence struct {
	FragmentID        string    `json:"fragment_id"`
	Speaker           string    `json:"speaker,omitempty"`
	SpanStart         int       `json:"span_start"`
	SpanEnd           int       `json:"span_end"`
	ExtractConf       float64   `json:"extract_conf"`
	ExtractionModel   string    `json:"extraction_model,omitempty"`
	ExtractionVersion string    `json:"extraction_version,omitempty"`
	PipelineRunID     string    `json:"pipeline_run_id,omitempty"`
	Authority         Authority `json:"authority,omitempty"`
}
