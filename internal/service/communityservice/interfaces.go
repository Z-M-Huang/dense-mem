// Package communityservice provides community detection over the knowledge
// graph using the Neo4j Graph Data Science (GDS) plugin.
//
// Profile isolation invariant: every method on every service interface in this
// package receives profileID as an explicit parameter. Implementations MUST
// scope all GDS graph projections to a profile-namespaced graph name so
// communities from different profiles are never mixed.
//
// GDS availability: the system MUST NOT fail at startup when GDS is absent.
// Use ProbeGDS to check availability and degrade gracefully when it returns
// false.
package communityservice

import "context"

// DetectCommunityService defines the interface for running graph community
// detection using the Neo4j Graph Data Science plugin.
//
// Implementations project the profile's knowledge graph into GDS memory, run
// a community detection algorithm (e.g. Louvain or Weakly Connected
// Components), and write the resulting community identifiers back to the graph
// as node properties.
//
// Returns ErrCommunityUnavailable when GDS is not installed.
// Returns ErrCommunityGraphTooLarge when the projection exceeds memory limits.
type DetectCommunityService interface {
	// Detect runs community detection for the given profile's knowledge graph.
	// It writes community membership back to each node as a property and
	// returns an error when detection cannot complete.
	Detect(ctx context.Context, profileID string) error
}
