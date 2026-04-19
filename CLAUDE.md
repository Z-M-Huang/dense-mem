# CLAUDE.md - Dense-Mem Coding Standards

## The Golden Rules

- **Be absolutely certain** before proposing changes
- **Be brutally honest** instead of vague or agreeable
- **Never assume** — verify or ask
- **Never cut corners** — doing it right beats doing it fast
- **Understand before modifying** — read first, change second

## Before Every Action

- Read and understand existing code before modifying
- State what you plan to do and why before doing it
- Check for existing functions, patterns, and utilities before creating new ones
- When multiple valid approaches exist, present them and ask

## Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26 |
| HTTP | echo v5 |
| ORM | GORM (Postgres driver) |
| Validation | go-playground/validator v10 |
| Graph DB | neo4j-go-driver v5 |
| Cache | go-redis v9 |

## Profile Isolation (CRITICAL)

Every database operation **MUST** filter by `profile_id`:

- **Neo4j**: Add `{profile_id: $profileId}` to all node patterns
- **Postgres**: Include `profile_id` WHERE clause or rely on RLS
- **Redis**: Prefix all keys with `profile:{id}:`

NEVER allow cross-profile data access.

## Service Layer Pattern

```
API Routes → Services → Database Clients
     ↓           ↓            ↓
Middleware  Business Logic  Raw Queries
```

## Error Handling

- Use typed errors, not strings
- Never expose internal details to clients
- Log full context internally
- Return appropriate HTTP status codes

## Testing Requirements

- **Unit**: Service layer logic with mocked DB calls
- **Integration**: Real database operations
- **E2E**: Full HTTP flow with profile isolation verification
- **Security**: Cross-profile access attempts must fail

## Security Requirements

- Validate all input with struct validator
- Parameterize all database queries
- Never log secrets or API keys
- Use constant-time comparison for API keys
- Rate limit per profile

## Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Packages | lowercase, no underscores | `profileservice` |
| Files | lowercase, underscores | `profile_service.go` |
| Types/Structs | PascalCase | `ProfileService` |
| Functions | PascalCase (exported), camelCase (private) | `GetProfile`, `getProfile` |
| Constants | PascalCase | `MaxQueryDepth` |
| Database | snake_case | `profile_id`, `created_at` |

## Documentation

- Document non-obvious invariants
- Mark intentional security decisions
- Explain architectural choices

## Knowledge Pipeline Contracts

The knowledge pipeline has stable contracts that all consumers MUST follow.
See `docs/knowledge-pipeline-contracts.md` for the full reference.

| Stage | Route | AC |
|-------|-------|-----|
| Fragment ingestion | `POST /api/v1/fragments` | AC-3 |
| Claim creation | `POST /api/v1/claims` | AC-16 |
| Claim verification | `POST /api/v1/claims/{id}/verify` | AC-28 |
| Fact promotion | `POST /api/v1/claims/{id}/promote` | AC-41 |
| Fragment retraction | `POST /api/v1/fragments/{id}/retract` | AC-48 |
| Community detection | `POST /api/v1/admin/profiles/{id}/community/detect` | AC-49 |
| Hybrid recall | `GET /api/v1/recall` | AC-55, AC-62 |

**Recall tier contract** (MUST NOT be changed without an ADR):
- Tier `"1"` = active facts (score = `truth_score`)
- Tier `"1.5"` = validated claims only (score = `extract_conf * 0.5`)
- Tier `"2"` = fragments (score = RRF)

Related documents:
- `docs/knowledge-pipeline-contracts.md` — stable schemas and error contracts
- `docs/knowledge-pipeline-operability.md` — RBAC, alerts, rollback, risk register
- `docs/knowledge-pipeline-client-contracts.md` — client/UI integration guide
- `docs/adr/` — architectural decision records
- `docs/policies/` — access control and classification policies