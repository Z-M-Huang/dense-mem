# Profile Isolation (CRITICAL)

These rules are CRITICAL. Violation causes data leakage between profiles.

## Core Rule

**Every database operation MUST filter by `profile_id`.**

This is non-negotiable. No exceptions.

## Enforcement by Store

### Neo4j

Every query must include `profile_id` filter:

```
MATCH (n {profile_id: $profileId})
WHERE n.profile_id = $profileId
```

- Add to node patterns
- Add to WHERE clauses
- Check relationship traversals don't cross boundaries
- One missed filter = tenant escape

### Postgres

- RLS policies apply automatically
- But explicit checks preferred for safety
- All tables have `profile_id` column

### Redis

Redis is optional for single-node deployments and required for multi-instance setups.
- All keys use prefix: `profile:{profileId}:...`
- Rate limit keys: `profile:{id}:ratelimit:...`
- SSE stream keys: `profile:{id}:stream:...`

## What This Means

When writing code:

- Service methods receive `profileId` as first parameter
- Repository/DB helpers require `profileId`
- Never allow optional profileId
- Never query without the filter

## Testing Requirement

Every feature must have:
- Cross-profile access test that FAILS
- Verify data from profile A cannot be read by profile B