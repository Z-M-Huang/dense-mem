# ADR-003: Profile Isolation for Relationship Traversals

**Status**: Accepted  
**Date**: 2026-04-19

## Context

Neo4j relationship traversals (e.g., `SUPPORTED_BY` edges from Claim to Fragment,
`PROMOTES_TO` from Claim to Fact) cross node boundaries. A misconfigured traversal
could follow a relationship into a node belonging to a different profile — a tenant
escape.

## Decision

All relationship traversal queries carry the `profile_id` filter on **every node
pattern**, not just the anchor node. The `ProfileScopeEnforcer` injects a
`$profileId` parameter into all `ScopedRead` and `ScopedWrite` calls.

Example — correct:
```cypher
MATCH (c:Claim {profile_id: $profileId, claim_id: $claimId})
      -[:SUPPORTED_BY]->(f:SourceFragment {profile_id: $profileId})
RETURN f
```

Example — incorrect (missing profile filter on fragment):
```cypher
MATCH (c:Claim {profile_id: $profileId, claim_id: $claimId})
      -[:SUPPORTED_BY]->(f:SourceFragment)  -- missing profile filter
RETURN f
```

Cross-profile relationships are physically impossible in the data model because:
1. All writes use `MERGE ... {profile_id: $profileId}` on both endpoints
2. The `ProfileScopeEnforcer` wraps every session

Defense-in-depth: the recall service additionally post-filters semantic search
results by `ProfileID` after the query returns.

## Consequences

- Every Cypher query is slightly more verbose but profile escape is impossible
  even if a future query is written without the enforcer wrapper.
- Integration tests MUST include a cross-profile isolation scenario that asserts
  no data bleeds between profiles.
