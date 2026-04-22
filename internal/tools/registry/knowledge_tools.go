package registry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

// --- post_claim -----------------------------------------------------------

func postClaimTool(deps Dependencies) Tool {
	available := deps.ClaimCreate != nil
	return Tool{
		Name:        "post_claim",
		Description: "Extract and persist a new Claim from supporting SourceFragments within the caller's profile. Returns the created claim with its initial candidate status.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"supported_by"},
			"properties": map[string]any{
				"supported_by": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "format": "uuid"},
					"minItems":    1,
					"description": "Fragment IDs that support this claim.",
				},
				"subject":         schemaString("Claim subject.", 256),
				"predicate":       schemaString("Claim predicate.", 128),
				"object":          schemaString("Claim object.", 1024),
				"modality":        schemaEnum([]string{"assertion", "question", "proposal", "speculation", "quoted"}),
				"polarity":        schemaEnum([]string{"+", "-"}),
				"speaker":         schemaString("Speaker of the claim.", 256),
				"extract_conf":    map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": "Extraction confidence [0,1]."},
				"resolution_conf": map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": "Entity resolution confidence [0,1]."},
				"idempotency_key": schemaString("Client-supplied dedupe key (scoped to profile).", 128),
				"valid_from":      map[string]any{"type": "string", "format": "date-time"},
				"valid_to":        map[string]any{"type": "string", "format": "date-time"},
			},
			"additionalProperties": false,
		},
		OutputSchema:   claimObjectSchema(),
		RequiredScopes: []string{"write"},
		Available:      available,
		Invoke:         postClaimInvoker(deps.ClaimCreate, available),
	}
}

func postClaimInvoker(svc claimservice.CreateClaimService, available bool) ToolInvoker {
	return func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
		if !available || svc == nil {
			return nil, ErrToolUnavailable
		}
		var claim domain.Claim
		if err := remapInput(input, &claim); err != nil {
			return nil, fmt.Errorf("post_claim: invalid input: %w", err)
		}
		res, err := svc.Create(ctx, profileID, &claim)
		if err != nil {
			return nil, err
		}
		out, err := structToMap(res.Claim)
		if err != nil {
			return nil, err
		}
		if res.Duplicate {
			out["duplicate"] = true
			out["duplicate_of"] = res.DuplicateOf
		}
		return out, nil
	}
}

// --- get_claim -----------------------------------------------------------

func getClaimTool(deps Dependencies) Tool {
	available := deps.ClaimGet != nil
	return Tool{
		Name:        "get_claim",
		Description: "Fetch a single Claim by ID within the caller's profile scope.",
		InputSchema: map[string]any{
			"type":                 "object",
			"required":             []string{"id"},
			"properties":           map[string]any{"id": schemaString("Claim ID.", 128)},
			"additionalProperties": false,
		},
		OutputSchema:   claimObjectSchema(),
		RequiredScopes: []string{"read"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			id, _ := input["id"].(string)
			if id == "" {
				return nil, errors.New("get_claim: id is required")
			}
			claim, err := deps.ClaimGet.Get(ctx, profileID, id)
			if err != nil {
				return nil, err
			}
			return structToMap(claim)
		},
	}
}

// --- list_claims ----------------------------------------------------------

func listClaimsTool(deps Dependencies) Tool {
	available := deps.ClaimList != nil
	return Tool{
		Name:        "list_claims",
		Description: "List claims for the caller's profile with optional status/modality filters and offset pagination.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"cursor": schemaString("Pagination cursor from a previous response.", 256),
			},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items":       map[string]any{"type": "array", "items": claimObjectSchema()},
				"next_cursor": map[string]any{"type": "string"},
				"has_more":    map[string]any{"type": "boolean"},
				"total":       map[string]any{"type": "integer"},
			},
		},
		RequiredScopes: []string{"read"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			limit := 20
			if v, ok := input["limit"].(float64); ok {
				limit = int(v)
			}
			claims, total, err := deps.ClaimList.List(ctx, profileID, limit, 0)
			if err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(claims))
			for _, c := range claims {
				m, err := structToMap(c)
				if err != nil {
					return nil, err
				}
				items = append(items, m)
			}
			return map[string]any{
				"items":       items,
				"next_cursor": "",
				"has_more":    false,
				"total":       total,
			}, nil
		},
	}
}

// --- verify_claim ---------------------------------------------------------

func verifyClaimTool(deps Dependencies) Tool {
	available := deps.ClaimVerify != nil
	return Tool{
		Name:        "verify_claim",
		Description: "Run entailment verification for a Claim within the caller's profile. Transitions status from candidate to validated or rejected.",
		InputSchema: map[string]any{
			"type":                 "object",
			"required":             []string{"id"},
			"properties":           map[string]any{"id": schemaString("Claim ID to verify.", 128)},
			"additionalProperties": false,
		},
		OutputSchema:   claimObjectSchema(),
		RequiredScopes: []string{"write"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			id, _ := input["id"].(string)
			if id == "" {
				return nil, errors.New("verify_claim: id is required")
			}
			claim, err := deps.ClaimVerify.Verify(ctx, profileID, id)
			if err != nil {
				return nil, err
			}
			return structToMap(claim)
		},
	}
}

// --- promote_claim --------------------------------------------------------

func promoteClaimTool(deps Dependencies) Tool {
	available := deps.FactPromote != nil
	return Tool{
		Name:        "promote_claim",
		Description: "Promote a validated Claim to a Fact within the caller's profile. The Claim must have status=validated and verdict=entailed.",
		InputSchema: map[string]any{
			"type":                 "object",
			"required":             []string{"claim_id"},
			"properties":           map[string]any{"claim_id": schemaString("ID of the validated Claim to promote.", 128)},
			"additionalProperties": false,
		},
		OutputSchema:   factObjectSchema(),
		RequiredScopes: []string{"write"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			claimID, _ := input["claim_id"].(string)
			if claimID == "" {
				return nil, errors.New("promote_claim: claim_id is required")
			}
			fact, err := deps.FactPromote.Promote(ctx, profileID, claimID)
			if err != nil {
				return nil, err
			}
			return structToMap(fact)
		},
	}
}

// --- get_fact -------------------------------------------------------------

func getFactTool(deps Dependencies) Tool {
	available := deps.FactGet != nil
	return Tool{
		Name:        "get_fact",
		Description: "Fetch a single Fact by ID within the caller's profile scope.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]any{
				"id":               schemaString("Fact ID.", 128),
				"valid_at":         map[string]any{"type": "string", "format": "date-time"},
				"known_at":         map[string]any{"type": "string", "format": "date-time"},
				"include_evidence": map[string]any{"type": "boolean"},
			},
			"additionalProperties": false,
		},
		OutputSchema:   factObjectSchema(),
		RequiredScopes: []string{"read"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			var req struct {
				ID              string     `json:"id"`
				ValidAt         *time.Time `json:"valid_at"`
				KnownAt         *time.Time `json:"known_at"`
				IncludeEvidence bool       `json:"include_evidence"`
			}
			if err := remapInput(input, &req); err != nil {
				return nil, fmt.Errorf("get_fact: invalid input: %w", err)
			}
			if req.ID == "" {
				return nil, errors.New("get_fact: id is required")
			}
			fact, err := deps.FactGet.Get(ctx, profileID, req.ID)
			if err != nil {
				return nil, err
			}
			if !factMatchesTemporalWindow(fact, req.ValidAt, req.KnownAt) {
				return nil, factservice.ErrFactNotFound
			}
			if !req.IncludeEvidence {
				factCopy := *fact
				factCopy.Evidence = nil
				fact = &factCopy
			}
			return structToMap(fact)
		},
	}
}

// --- list_facts -----------------------------------------------------------

func listFactsTool(deps Dependencies) Tool {
	available := deps.FactList != nil
	return Tool{
		Name:        "list_facts",
		Description: "List facts for the caller's profile with optional filters and keyset pagination.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":            map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
				"cursor":           schemaString("Pagination cursor from a previous response.", 256),
				"subject":          schemaString("Filter by subject.", 256),
				"predicate":        schemaString("Filter by predicate.", 128),
				"status":           schemaEnum([]string{"active", "retracted", "superseded", "needs_revalidation"}),
				"valid_at":         map[string]any{"type": "string", "format": "date-time"},
				"known_at":         map[string]any{"type": "string", "format": "date-time"},
				"include_evidence": map[string]any{"type": "boolean"},
			},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items":       map[string]any{"type": "array", "items": factObjectSchema()},
				"next_cursor": map[string]any{"type": "string"},
				"has_more":    map[string]any{"type": "boolean"},
			},
		},
		RequiredScopes: []string{"read"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			var req struct {
				Limit           int        `json:"limit"`
				Cursor          string     `json:"cursor"`
				Subject         string     `json:"subject"`
				Predicate       string     `json:"predicate"`
				Status          string     `json:"status"`
				ValidAt         *time.Time `json:"valid_at"`
				KnownAt         *time.Time `json:"known_at"`
				IncludeEvidence bool       `json:"include_evidence"`
			}
			if err := remapInput(input, &req); err != nil {
				return nil, fmt.Errorf("list_facts: invalid input: %w", err)
			}
			limit := req.Limit
			if limit == 0 {
				limit = 20
			}
			filters := factservice.FactListFilters{
				Subject:   req.Subject,
				Predicate: req.Predicate,
				Status:    domain.FactStatus(req.Status),
				ValidAt:   req.ValidAt,
				KnownAt:   req.KnownAt,
			}
			facts, nextCursor, err := deps.FactList.List(ctx, profileID, filters, limit, req.Cursor)
			if err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(facts))
			for _, f := range facts {
				if !req.IncludeEvidence {
					factCopy := *f
					factCopy.Evidence = nil
					f = &factCopy
				}
				m, err := structToMap(f)
				if err != nil {
					return nil, err
				}
				items = append(items, m)
			}
			return map[string]any{
				"items":       items,
				"next_cursor": nextCursor,
				"has_more":    nextCursor != "",
			}, nil
		},
	}
}

// --- retract_fragment -----------------------------------------------------

func retractFragmentTool(deps Dependencies) Tool {
	available := deps.FragmentRetract != nil
	return Tool{
		Name:        "retract_fragment",
		Description: "Tombstone a SourceFragment and recompute affected facts within the caller's profile. The fragment is soft-deleted; graph lineage is preserved.",
		InputSchema: map[string]any{
			"type":                 "object",
			"required":             []string{"id"},
			"properties":           map[string]any{"id": schemaString("Fragment ID to retract.", 128)},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		RequiredScopes: []string{"write"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			id, _ := input["id"].(string)
			if id == "" {
				return nil, errors.New("retract_fragment: id is required")
			}
			if err := deps.FragmentRetract.Retract(ctx, profileID, id); err != nil {
				return nil, err
			}
			return map[string]any{}, nil
		},
	}
}

// --- detect_community -----------------------------------------------------

func detectCommunityTool(deps Dependencies) Tool {
	available := deps.CommunityDetect != nil
	return Tool{
		Name:        "detect_community",
		Description: "Run graph community detection for the caller's profile using the Neo4j Graph Data Science plugin. Writes community IDs back to graph nodes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"gamma":      map[string]any{"type": "number", "minimum": 0, "description": "Louvain resolution parameter. Defaults to 1.0."},
				"tolerance":  map[string]any{"type": "number", "minimum": 0, "description": "Convergence threshold for iterative algorithms."},
				"max_levels": map[string]any{"type": "integer", "minimum": 1, "description": "Maximum hierarchical merge levels."},
			},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		RequiredScopes: []string{"write"},
		Available:      available,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			if !available {
				return nil, ErrToolUnavailable
			}
			if err := deps.CommunityDetect.Detect(ctx, profileID); err != nil {
				return nil, err
			}
			return map[string]any{}, nil
		},
	}
}

func factMatchesTemporalWindow(f *domain.Fact, validAt, knownAt *time.Time) bool {
	if f == nil {
		return false
	}
	if validAt != nil {
		if f.ValidFrom != nil && f.ValidFrom.After(*validAt) {
			return false
		}
		if f.ValidTo != nil && !f.ValidTo.After(*validAt) {
			return false
		}
	}
	if knownAt != nil {
		if f.RecordedAt.After(*knownAt) {
			return false
		}
		if f.RecordedTo != nil && !f.RecordedTo.After(*knownAt) {
			return false
		}
	}
	return true
}

// --- schema helpers -------------------------------------------------------

// claimObjectSchema mirrors dto.ClaimResponse. Hand-built to avoid reflection.
func claimObjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"claim_id":           map[string]any{"type": "string"},
			"profile_id":         map[string]any{"type": "string"},
			"subject":            map[string]any{"type": "string"},
			"predicate":          map[string]any{"type": "string"},
			"object":             map[string]any{"type": "string"},
			"modality":           map[string]any{"type": "string"},
			"polarity":           map[string]any{"type": "string"},
			"speaker":            map[string]any{"type": "string"},
			"span_start":         map[string]any{"type": "integer"},
			"span_end":           map[string]any{"type": "integer"},
			"valid_from":         map[string]any{"type": "string", "format": "date-time"},
			"valid_to":           map[string]any{"type": "string", "format": "date-time"},
			"recorded_at":        map[string]any{"type": "string", "format": "date-time"},
			"recorded_to":        map[string]any{"type": "string", "format": "date-time"},
			"extract_conf":       map[string]any{"type": "number"},
			"resolution_conf":    map[string]any{"type": "number"},
			"source_quality":     map[string]any{"type": "number"},
			"entailment_verdict": map[string]any{"type": "string"},
			"status":             map[string]any{"type": "string"},
			"extraction_model":   map[string]any{"type": "string"},
			"extraction_version": map[string]any{"type": "string"},
			"verifier_model":     map[string]any{"type": "string"},
			"pipeline_run_id":    map[string]any{"type": "string"},
			"content_hash":       map[string]any{"type": "string"},
			"idempotency_key":    map[string]any{"type": "string"},
			"classification":     map[string]any{"type": "object"},
			"supported_by":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"evidence":           map[string]any{"type": "array", "items": evidenceObjectSchema()},
		},
	}
}

// factObjectSchema mirrors dto.FactResponse. Hand-built to avoid reflection.
func factObjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"fact_id":                        map[string]any{"type": "string"},
			"profile_id":                     map[string]any{"type": "string"},
			"subject":                        map[string]any{"type": "string"},
			"predicate":                      map[string]any{"type": "string"},
			"object":                         map[string]any{"type": "string"},
			"status":                         map[string]any{"type": "string"},
			"truth_score":                    map[string]any{"type": "number"},
			"valid_from":                     map[string]any{"type": "string", "format": "date-time"},
			"valid_to":                       map[string]any{"type": "string", "format": "date-time"},
			"recorded_at":                    map[string]any{"type": "string", "format": "date-time"},
			"recorded_to":                    map[string]any{"type": "string", "format": "date-time"},
			"retracted_at":                   map[string]any{"type": "string", "format": "date-time"},
			"last_confirmed_at":              map[string]any{"type": "string", "format": "date-time"},
			"promoted_from_claim_id":         map[string]any{"type": "string"},
			"classification":                 map[string]any{"type": "object"},
			"classification_lattice_version": map[string]any{"type": "string"},
			"source_quality":                 map[string]any{"type": "number"},
			"labels":                         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"metadata":                       map[string]any{"type": "object"},
			"evidence":                       map[string]any{"type": "array", "items": evidenceObjectSchema()},
		},
	}
}
