# Policy: Classification and Handling of Knowledge Artifacts

## Scope

This policy governs how knowledge artifacts (Fragments, Claims, Facts) are
classified and handled based on their content sensitivity and pipeline stage.

## Classification Tiers

| Tier | Label | Examples | Handling |
|------|-------|----------|----------|
| T1 | Public | Publicly available facts, citations | No special handling required |
| T2 | Internal | Personal notes, private context | Profile-isolated storage; never exposed to other profiles |
| T3 | Sensitive | Credentials, PII, health data | Never store as fragment content; caller responsibility |

Dense-mem does not inspect content for sensitivity classification. All content
is treated as at minimum T2 (internal). Callers are responsible for not
submitting T3 content.

## Handling Requirements

### Fragment Content
- Stored as a string property on `SourceFragment` nodes in Neo4j
- Encrypted at rest via the database encryption layer (infrastructure concern)
- Never logged — `content` fields are excluded from audit log payloads
- Never returned in error messages

### Claim Content
- Derived from fragment content by the caller (not by dense-mem server-side)
- Same storage and logging rules as fragments

### Fact Content
- Promoted from validated claims; inherited content classification
- `truth_score` and `predicate` fields are stored unencrypted as graph
  properties (graph traversal requires them to be plaintext)

## Retention

Knowledge artifacts are retained until explicitly deleted or retracted by the
profile owner or an admin. There is no automatic expiry.

Retracted fragments remain as soft tombstones in the graph indefinitely to
preserve lineage. Hard deletion removes the node and all incident edges.

## Audit

All mutations (create, delete, retract, verify, promote) emit an `AuditLogEntry`.
The `after_payload` field captures the entity state post-mutation but MUST NOT
include raw fragment content or claim text.
