# Knowledge Pipeline — Operability Guide

## RBAC Matrix

| Actor | Fragments | Claims | Facts | Recall | Community | Maintenance |
|------|-----------|--------|-------|--------|-----------|-------------|
| Standard API Key (read scope) | read, list | read, list | read, list | query | list, get | — |
| Standard API Key (write scope) | create, delete, retract | create, delete, verify, promote | — | — | — | — |
| Operator command | — | — | — | — | — | profile lifecycle, key rotation, invariant scans, migrations |

## Alert Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Recall latency p99 | > 2 s | > 5 s | Check embedding provider / Neo4j GDS |
| Embedding error rate | > 1% | > 5% | Check AI provider key / quota |
| Fragment create error rate | > 0.5% | > 2% | Check Neo4j write availability |
| Claim verify timeout rate | > 2% | > 10% | Check verifier provider; apply backoff |
| Promotion lock contention | > 5% | > 20% | Check Postgres advisory lock TTL |

## Audit Retention

All operations that mutate pipeline state emit an `AuditLogEntry` to the
`audit_log` Postgres table. Entries are append-only and immutable.

Recommended retention:

| Environment | Retention |
|-------------|-----------|
| Production | 365 days |
| Staging | 90 days |
| Development | 30 days |

## Rollback Procedures

### Fragment creation rollback
Fragments can be soft-tombstoned via `POST /api/v1/fragments/{id}/retract`.
Hard deletion is available via `DELETE /api/v1/fragments/{id}` with `write` scope.

### Claim rollback
Claims can be hard-deleted via `DELETE /api/v1/claims/{id}`. This is irreversible.
A deleted claim does not affect promoted facts (they are independent nodes).

### Fact rollback
Facts are created during claim promotion. There is no dedicated fact-delete endpoint.
To invalidate a fact, retract the supporting fragment — this triggers revalidation and
may transition the fact to `needs_revalidation` status.

### Community detection rollback
Community assignments are stored as `community_id` properties on Claim/Fact nodes.
Re-running detection with a different algorithm overwrites existing assignments.

## Lattice Versioning Policy

The knowledge-pipeline lattice schema is versioned via Neo4j constraint names.
Breaking changes (e.g., new uniqueness constraints, index dimension changes) require:
1. A migration script applied before the new binary is deployed
2. An ADR documenting the constraint change and backward compatibility window
3. A smoke test that verifies the new constraint is in place before accepting traffic

Non-breaking changes (new optional properties, new relationship types) can be
deployed without a migration step.

## Evidence Collection Matrix

| AC | Verified By | Gate |
|----|-------------|------|
| AC-55 recall HTTP route | `TestRecallHandler` | go test handler |
| AC-62 tier expansion | `TestRecallHandler` + tier field assertions | go test handler |
| AC-63 cross-profile isolation | `TestRecallHandler_CrossProfileIsolation` | go test handler |
| AC-64 bootstrap wiring | `TestServerBootstrapWiresKnowledgePipeline` | go test server |
| AC-65 build | `go build ./...` | CI |
| AC-66 vet | `go vet ./...` | CI |

## Added Risks

| Risk | ID | Severity | Mitigation |
|------|----|----------|------------|
| Claim weight misconfiguration causes facts to rank below claims | RK-13 | Medium | DefaultRecallValidatedClaimWeight is a named constant; test coverage asserts tier "1" outranks "1.5" for equal base scores |
| Tier enrichment failure silently degrades recall quality | RK-14 | Low | enrichTierHits logs and swallows errors; callers still get fragment results |
| Large community detection run blocks Neo4j GDS write lock | RK-15 | High | CommunityDetectRequest requires explicit tuning parameters; tool execution should enforce GDS timeout |
