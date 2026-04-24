---
paths:
  - "internal/storage/**/*.go"
  - "internal/repository/**/*.go"
  - "internal/service/**/*.go"
---

# Database Access Rules

Rules for database clients and the service layer.

## Neo4j Queries

Always use parameterized queries via the `neo4j-go-driver/v5` session APIs:

```go
result, err := session.Run(ctx,
    "MATCH (f:Fact {profile_id: $profileId}) WHERE f.status = $status RETURN f",
    map[string]any{"profileId": profileID, "status": status})
```

Never concatenate variables into Cypher strings.

## Postgres Queries

Use GORM with bound parameters, or `db.Raw`/`db.Exec` with placeholders:

```go
db.WithContext(ctx).Raw(
    "SELECT * FROM facts WHERE profile_id = ?", profileID,
).Scan(&facts)
```

RLS (`SET LOCAL app.current_profile_id = ...` via `set_config`) is enforced
inside `WithProfileTx` / `WithSystemTx` in `internal/storage/postgres/rls.go`.
All repo calls that cross a trust boundary must run through those helpers.

## Redis Keys

Always use prefixed keys with the current live prefixes.
Redis is optional for single-node deployments and required for multi-instance setups.

```go
rateKey := fmt.Sprintf("profile:%s:ratelimit:%s", profileID, key)
streamKey := fmt.Sprintf("profile:%s:stream:%s", profileID, requestID)
```

Never use unprefixed keys.

## Service Layer Pattern

Services are the only layer that calls repository / DB clients.

- HTTP handlers call services
- Services call repositories
- Repositories execute DB queries

Services inject `profileID` into every downstream call.

## Connection Pooling

- Neo4j: single `neo4j.DriverWithContext` with built-in connection pool
- Postgres: `gorm.io/driver/postgres` (pgx under the hood) — pool configured at
  startup in `internal/storage/postgres/client.go`
- Redis: single `*redis.Client` with its default pool

## Error Handling

DB errors should be:
- Logged with full context
- Converted to typed errors (see `internal/httperr`)
- Never expose internals to the client
