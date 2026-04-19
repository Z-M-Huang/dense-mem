package factservice

// Policy defines how fact promotion behaves for a given predicate.
//
// R2 (binding): All four policy values are defined here. Only SingleCurrent
// and MultiValued are implemented in v1. Attempting to use Versioned or
// AppendOnly returns ErrUnsupportedPolicy.
type Policy string

const (
	// SingleCurrent allows exactly one active Fact per (subject, predicate)
	// pair within a profile. Promoting a new claim triggers contradiction
	// resolution against existing active facts.
	SingleCurrent Policy = "single_current"

	// MultiValued allows multiple coexisting active Facts for the same
	// (subject, predicate) pair. No contradiction logic runs.
	MultiValued Policy = "multi_valued"

	// Versioned is defined by the spec but is not implemented in v1.
	// Promote calls on predicates with this policy return ErrUnsupportedPolicy.
	Versioned Policy = "versioned"

	// AppendOnly is defined by the spec but is not implemented in v1.
	// Promote calls on predicates with this policy return ErrUnsupportedPolicy.
	AppendOnly Policy = "append_only"
)

// PromotionGate holds per-predicate thresholds and constraints that must ALL
// be satisfied before a validated Claim is eligible for promotion to a Fact.
//
// Support gate semantics (OR, not AND): a claim passes the support check when
// EITHER support_count >= MinSourceCount OR max_source_quality >= MinMaxSourceQuality.
// Treating these as AND is a known wrong implementation (AC-35).
type PromotionGate struct {
	// Policy determines contradiction behaviour for this predicate.
	Policy Policy

	// MinExtractConf is the minimum extract_conf score the claim must carry.
	MinExtractConf float64

	// MinResolutionConf is the minimum resolution_conf score the claim must carry.
	MinResolutionConf float64

	// RequiresAssertion, when true, demands that the claim's Modality is
	// ModalityAssertion. Belief/hypothesis/intention claims are rejected.
	RequiresAssertion bool

	// RequiresEntailed, when true, demands that the claim's EntailmentVerdict
	// is VerdictEntailed. Neutral/contradicted/unverified claims are rejected.
	RequiresEntailed bool

	// MinSourceCount is the minimum number of supporting SourceFragments for
	// the support gate. See support gate semantics above.
	MinSourceCount int

	// MinMaxSourceQuality is the alternative support gate threshold: if the
	// highest source quality among supporting fragments reaches this value, the
	// support gate passes even when support_count < MinSourceCount. Set to 0.0
	// to disable this alternative path.
	MinMaxSourceQuality float64
}

// DefaultPromotionGates is the authoritative, read-only gate map for the v1
// shipping predicate set (AC-34). Any predicate not present in this map is
// denied at promotion time with ErrPredicateNotPoliced (strict deny-by-default).
//
// Security invariant: this map is initialised once at package init and must
// never be mutated at runtime. Callers that need to look up a gate MUST treat
// the returned value as a copy.
//
// Gate map (binding — from requirements rev.2):
//
//	Predicate  | Policy        | ExtConf | ResConf | Assertion | Entailed | MinSrc | MinMaxQ
//	-----------|---------------|---------|---------|-----------|----------|--------|--------
//	born_on    | single_current|  0.90   |  0.80   |   true    |   true   |   1    |  0.95
//	died_on    | single_current|  0.90   |  0.80   |   true    |   true   |   1    |  0.95
//	works_at   | single_current|  0.85   |  0.75   |   true    |   true   |   2    |  0.95
//	likes      | multi_valued  |  0.70   |  0.60   |   false   |   true   |   1    |  0.00
//	knows      | multi_valued  |  0.75   |  0.60   |   true    |   true   |   1    |  0.00
//	has_skill  | multi_valued  |  0.80   |  0.60   |   true    |   true   |   1    |  0.00
var DefaultPromotionGates = map[string]PromotionGate{
	"born_on":   {SingleCurrent, 0.9, 0.8, true, true, 1, 0.95},
	"died_on":   {SingleCurrent, 0.9, 0.8, true, true, 1, 0.95},
	"works_at":  {SingleCurrent, 0.85, 0.75, true, true, 2, 0.95},
	"likes":     {MultiValued, 0.7, 0.6, false, true, 1, 0.0},
	"knows":     {MultiValued, 0.75, 0.6, true, true, 1, 0.0},
	"has_skill": {MultiValued, 0.8, 0.6, true, true, 1, 0.0},
}
