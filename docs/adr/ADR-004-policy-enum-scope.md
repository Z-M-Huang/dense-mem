# ADR-004: Policy Enum Scope

**Status**: Accepted  
**Date**: 2026-04-19

## Context

Claim promotion applies a *policy* to decide whether a new fact should replace,
coexist with, or be rejected in favour of an existing comparable fact. The policy
enum must be:
1. Exhaustive — every promotion must map to exactly one policy
2. Backward-compatible — adding a new policy must not break existing stored data
3. Validated at the API boundary — invalid policy strings must be rejected before
   reaching the service layer

## Decision

The promotion policy is an **explicit string enum** stored as a Neo4j string
property. Currently accepted values:

| Policy | Behaviour |
|--------|-----------|
| `REPLACE` | New fact replaces an existing comparable fact |
| `COEXIST` | New fact is created alongside any existing comparable fact |
| `REJECT_IF_WEAKER` | Promotion is rejected if an existing comparable fact has a higher `truth_score` |

The default policy when not supplied is `REJECT_IF_WEAKER`.

New enum values are added by:
1. Updating the Go enum type in `internal/domain/`
2. Adding handling in `factservice/promote.go`
3. Writing a migration note (not a schema migration — the property is a string)
4. Updating this ADR

Removed enum values are handled by returning a 422 for unknown values rather
than falling back to a default, so no silent behaviour change occurs.

## Consequences

- Invalid policy strings are caught at the struct-validator layer (go-playground
  `oneof` tag) before reaching the service.
- The API surface is explicit; clients cannot pass unvalidated strings.
- Adding a policy is backward-compatible. Removing one is a breaking change and
  requires a deprecation period with a documented migration path.
