# Policy: Knowledge Pipeline Access Control

## Scope

This policy governs who can read, write, verify, promote, and retract
knowledge-pipeline entities (Fragments, Claims, Facts) within dense-mem.

## Principals

| Principal | Authentication | Capabilities |
|-----------|---------------|--------------|
| Profile owner (standard key, write) | `Authorization: Bearer <key>` + `X-Profile-ID` | Create, delete, retract fragments; create, delete, verify, promote claims |
| Profile owner (standard key, read) | Same | Read and list fragments, claims, facts; recall |
| Admin | Admin API key | All profile operations; community detection; admin graph query; invariant scan |
| Unauthenticated | None | None — deny by default |

## Deny-by-Default

All endpoints require a valid API key. There are no public endpoints in the
knowledge pipeline. The OpenAPI spec endpoint (`/api/v1/openapi.json`) requires
authentication.

## Cross-Profile Access

A standard API key MUST only be used with the `X-Profile-ID` it was issued for.
Requests where the key's profile does not match the requested profile ID are
rejected with 403.

Admin keys may operate on any profile; all admin operations are audit-logged with
the admin actor's key ID.

## Principle of Least Privilege

Handlers check the required scope (`read` or `write`) before processing. Scopes
are attached to the API key at creation time and cannot be escalated.

The `recall` endpoint requires `read` scope only. Recall never mutates state.
