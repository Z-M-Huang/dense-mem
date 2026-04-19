# ADR-001: Promotion Serialization Boundary

**Status**: Accepted  
**Date**: 2026-04-19

## Context

Promoting a validated claim to a fact involves:
1. Reading the claim from Neo4j
2. Checking a gate (truth threshold, comparable fact, lattice policy)
3. Writing the new Fact node in Neo4j
4. Updating the claim's `status` to `promoted`

Concurrent promotions of the same claim could produce duplicate Fact nodes
or leave the claim in an inconsistent state.

## Decision

Serialize promotions per claim using a **Postgres advisory lock** keyed on the
claim's UUID. The lock is acquired before the Neo4j read and released after the
write completes. A `MERGE` on `(profile_id, fact_id)` in the Cypher write
provides an additional idempotency guard.

The lock timeout defaults to 30 seconds and is configurable via
`CONFIG_PROMOTE_TX_TIMEOUT_SEC`. Callers that cannot acquire the lock within the
timeout receive a 409 conflict response.

## Consequences

- Concurrent promotion requests for the same claim are serialized with at most
  one winner; all others receive 409.
- The advisory lock is profile-scoped (key incorporates `profile_id + claim_id`)
  so lock contention is isolated per profile — one busy profile does not block
  another.
- Cross-service lock release is not an issue because the lock is acquired and
  released within a single request lifetime.
