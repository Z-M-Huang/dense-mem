// Package openapi generates OpenAPI 3.0.3 specifications from the tool
// registry and a declarative route table. The spec is produced at runtime so
// it always reflects the live set of tools and routes; it is never hand
// edited (AC-34, AC-35).
package openapi

// RouteDescriptor is the static metadata for one HTTP route. The generator
// classifies routes by the AISafe / AdminOnly flags. Routes backed by a
// registered tool name inherit their request/response schemas from the
// registry rather than duplicating them here.
type RouteDescriptor struct {
	Method      string
	Path        string
	OperationID string
	// ToolName links to a registry entry. When set, the operation's request
	// body schema and 200-response schema are pulled directly from the
	// registry tool's InputSchema / OutputSchema (AC-34 "derived from
	// registry"). Empty for routes with no tool counterpart.
	ToolName string
	AISafe   bool
	// AdminOnly routes appear in the full variant only.
	AdminOnly   bool
	Description string

	// RequestSchema is an explicit component-schema name for the request body.
	// When set, it takes priority over any ToolName-derived input schema.
	// This allows routes that ship before MCP tool registration to declare
	// their schemas directly.
	RequestSchema string

	// ResponseSchema is an explicit component-schema name for the success
	// response body. When set, it takes priority over any ToolName-derived
	// output schema.
	ResponseSchema string

	// SuccessStatus is the HTTP status code for the success response. Defaults
	// to 200 when zero. Use 201 for resource-creation routes.
	SuccessStatus int

	// ExtraResponses maps additional HTTP status codes to their descriptions.
	// These are merged into the operation's responses block alongside the
	// standard error codes.
	ExtraResponses map[string]string

	// Tags overrides the auto-inferred operation tags when non-empty. If
	// empty, tags are derived from the route path and flags as before.
	Tags []string
}

// DefaultRoutes returns the canonical route table for the v1 surface. New
// routes registered in router_protected.go must also be added here so they
// surface in the generated OpenAPI doc.
func DefaultRoutes() []RouteDescriptor {
	return []RouteDescriptor{
		// --- Claims (AI-safe, knowledge pipeline Phase 2 & 3) ---
		{Method: "POST", Path: "/api/v1/claims", OperationID: "createClaim", RequestSchema: "ClaimRequest", ResponseSchema: "ClaimResponse", SuccessStatus: 201, AISafe: true, Tags: []string{"knowledge"}, Description: "Create a new candidate claim derived from a source fragment."},
		{Method: "GET", Path: "/api/v1/claims/{id}", OperationID: "getClaim", ResponseSchema: "ClaimResponse", AISafe: true, Tags: []string{"knowledge"}, Description: "Fetch a single claim by id."},
		{Method: "GET", Path: "/api/v1/claims", OperationID: "listClaims", AISafe: true, Tags: []string{"knowledge"}, Description: "List claims (keyset pagination)."},
		{Method: "DELETE", Path: "/api/v1/claims/{id}", OperationID: "deleteClaim", AISafe: true, Tags: []string{"knowledge"}, Description: "Hard-delete a claim."},
		// Phase 3: entailment verification (AC-28, AC-30)
		{Method: "POST", Path: "/api/v1/claims/{id}/verify", OperationID: "verifyClaim", ResponseSchema: "VerifyClaimResponse", AISafe: true, Tags: []string{"knowledge"}, Description: "Run entailment verification for a candidate claim.", ExtraResponses: map[string]string{
			"429": "Verifier rate-limited; retry after Retry-After seconds.",
			"502": "Verifier returned a malformed response.",
			"503": "Verifier provider unavailable.",
			"504": "Verifier request timed out.",
		}},
		// Phase 4: fact promotion (AC-41, AC-42)
		{Method: "POST", Path: "/api/v1/claims/{id}/promote", OperationID: "promoteClaim", ResponseSchema: "FactResponse", SuccessStatus: 201, AISafe: true, Tags: []string{"knowledge"}, Description: "Promote a validated claim to an authoritative fact.", ExtraResponses: map[string]string{
			"409": "Claim not validated, gate rejected, comparable fact disputed, or claim weaker than existing fact.",
			"422": "Predicate not policed or unsupported promotion policy.",
		}},

		// --- Facts (AI-safe, knowledge pipeline Phase 4) ---
		{Method: "GET", Path: "/api/v1/facts/{id}", OperationID: "getFact", ResponseSchema: "FactResponse", AISafe: true, Tags: []string{"knowledge"}, Description: "Fetch a single promoted fact by id."},
		{Method: "GET", Path: "/api/v1/facts", OperationID: "listFacts", AISafe: true, Tags: []string{"knowledge"}, Description: "List promoted facts (keyset pagination)."},

		// --- Fragments (AI-safe) ---
		{Method: "POST", Path: "/api/v1/fragments", OperationID: "createFragment", ToolName: "save_memory", AISafe: true, Description: "Save a new memory fragment."},
		{Method: "GET", Path: "/api/v1/fragments", OperationID: "listFragments", ToolName: "list_recent_memories", AISafe: true, Description: "List recent fragments (keyset pagination)."},
		{Method: "GET", Path: "/api/v1/fragments/{id}", OperationID: "getFragment", ToolName: "get_memory", AISafe: true, Description: "Fetch a single fragment by id."},
		{Method: "DELETE", Path: "/api/v1/fragments/{id}", OperationID: "deleteFragment", AISafe: true, Description: "Hard-delete a fragment."},
		// Phase 6: soft tombstone (AC-48)
		{Method: "POST", Path: "/api/v1/fragments/{id}/retract", OperationID: "retractFragment", ResponseSchema: "RetractFragmentResponse", AISafe: true, Tags: []string{"knowledge"}, Description: "Soft-tombstone a fragment; preserves graph lineage and triggers fact revalidation."},

		// --- Tool catalog (AI-safe) ---
		{Method: "GET", Path: "/api/v1/tools", OperationID: "listTools", AISafe: true, Description: "List all registered tools."},

		// --- Recall (AI-safe) ---
		{Method: "GET", Path: "/api/v1/recall", OperationID: "recallKnowledge", ResponseSchema: "RecallResponse", AISafe: true, Tags: []string{"knowledge"}, Description: "Hybrid semantic + keyword recall spanning all knowledge-pipeline tiers (facts, claims, fragments).", ExtraResponses: map[string]string{
			"422": "Missing or invalid q parameter.",
			"503": "Embedding provider unavailable.",
		}},
		{Method: "POST", Path: "/api/v1/tools/recall-memory", OperationID: "recallMemory", ToolName: "recall_memory", AISafe: true, Description: "Hybrid semantic + keyword recall over fragments."},

		// --- Advanced tool routes (full variant only) ---
		{Method: "POST", Path: "/api/v1/tools/graph-query", OperationID: "graphQueryTool", ToolName: "graph-query", Description: "Advanced: read-only Cypher query."},
		{Method: "POST", Path: "/api/v1/tools/keyword-search", OperationID: "keywordSearchTool", ToolName: "keyword-search", Description: "Advanced: BM25 keyword search."},
		{Method: "POST", Path: "/api/v1/tools/semantic-search", OperationID: "semanticSearchTool", ToolName: "semantic-search", Description: "Advanced: kNN vector search."},

		// --- Profile CRUD (full variant) ---
		{Method: "POST", Path: "/api/v1/profiles", OperationID: "createProfile", AdminOnly: true, Description: "Create a profile (admin)."},
		{Method: "GET", Path: "/api/v1/profiles", OperationID: "listProfiles", AdminOnly: true, Description: "List profiles (admin)."},
		{Method: "GET", Path: "/api/v1/profiles/{profileId}", OperationID: "getProfile", Description: "Get a profile."},
		{Method: "PATCH", Path: "/api/v1/profiles/{profileId}", OperationID: "patchProfile", Description: "Update profile metadata."},
		{Method: "DELETE", Path: "/api/v1/profiles/{profileId}", OperationID: "deleteProfile", AdminOnly: true, Description: "Delete a profile (admin)."},

		// --- Audit log (full variant) ---
		{Method: "GET", Path: "/api/v1/profiles/{profileId}/audit-log", OperationID: "getAuditLog", Description: "Fetch the profile's audit log."},

		// --- API keys (admin/full variant) ---
		{Method: "POST", Path: "/api/v1/profiles/{profileId}/api-keys", OperationID: "createAPIKey", AdminOnly: true, Description: "Create a new API key (admin)."},
		{Method: "GET", Path: "/api/v1/profiles/{profileId}/api-keys", OperationID: "listAPIKeys", AdminOnly: true, Description: "List API keys (admin)."},
		{Method: "DELETE", Path: "/api/v1/profiles/{profileId}/api-keys/{keyId}", OperationID: "deleteAPIKey", AdminOnly: true, Description: "Revoke an API key (admin)."},

		// --- SSE query stream (full variant) ---
		{Method: "POST", Path: "/api/v1/profiles/{profileId}/query/stream", OperationID: "queryStream", Description: "Server-sent event stream for long-running queries."},

		// --- Admin routes (full variant only) ---
		{Method: "GET", Path: "/api/v1/admin/stats", OperationID: "adminStats", AdminOnly: true, Description: "Admin operational stats."},
		{Method: "GET", Path: "/api/v1/admin/keys", OperationID: "adminListKeys", AdminOnly: true, Description: "List all API keys across profiles (admin)."},
		{Method: "POST", Path: "/api/v1/admin/graph/query", OperationID: "adminGraphQuery", AdminOnly: true, Description: "Admin: raw Cypher with profile-scope injection."},
		{Method: "POST", Path: "/api/v1/admin/invariant-scan", OperationID: "adminInvariantScan", AdminOnly: true, Description: "Admin: scan for invariant violations."},
		// Phase 7: community detection (AC-49, AC-50, AC-52, AC-53, AC-54)
		{Method: "POST", Path: "/api/v1/admin/profiles/{profileId}/community/detect", OperationID: "adminDetectCommunity", RequestSchema: "CommunityDetectRequest", AdminOnly: true, Tags: []string{"community"}, Description: "Admin: trigger community detection for a profile's knowledge graph using the Neo4j GDS plugin."},
	}
}
