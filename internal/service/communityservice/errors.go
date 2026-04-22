package communityservice

import "errors"

// ErrCommunityUnavailable is returned when the Neo4j Graph Data Science plugin
// is not installed or its procedures are not callable.
//
// This error MUST NOT be treated as fatal at startup — the system degrades
// gracefully when GDS is absent. Callers should log the condition and skip
// community detection rather than aborting.
//
// Callers may wrap this error: errors.Is(err, ErrCommunityUnavailable).
var ErrCommunityUnavailable = errors.New("community detection service unavailable")

// ErrCommunityGraphTooLarge is returned when the projected graph exceeds the
// GDS memory limit or an operator-configured node/relationship cap.
//
// Callers may wrap this error: errors.Is(err, ErrCommunityGraphTooLarge).
var ErrCommunityGraphTooLarge = errors.New("community graph too large")

// ErrCommunityNotFound is returned when a persisted community summary does not
// exist or belongs to a different profile.
var ErrCommunityNotFound = errors.New("community not found")
