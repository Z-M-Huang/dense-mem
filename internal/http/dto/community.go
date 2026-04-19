package dto

// CommunityDetectRequest is the request body for triggering community detection
// on a profile's knowledge graph via
// POST /api/v1/admin/profiles/:profileId/community/detect.
//
// All fields are optional tuning parameters. When omitted the service uses its
// built-in defaults. The current DetectCommunityService interface accepts only
// profileID; these params are declared for future service evolution and for
// OpenAPI documentation completeness.
type CommunityDetectRequest struct {
	// Gamma controls the resolution parameter for Louvain community detection.
	// Higher values produce more, smaller communities. Defaults to 1.0 when zero.
	Gamma float64 `json:"gamma,omitempty" validate:"omitempty,gte=0"`
	// Tolerance is the convergence threshold for iterative algorithms. Smaller
	// values increase precision at the cost of more iterations.
	Tolerance float64 `json:"tolerance,omitempty" validate:"omitempty,gte=0"`
	// MaxLevels caps the number of hierarchical community-merge levels.
	// Defaults to 10 when zero.
	MaxLevels int `json:"max_levels,omitempty" validate:"omitempty,gte=1"`
}
