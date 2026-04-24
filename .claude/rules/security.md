# Security Rules

Security requirements for all code.

## Input Validation

- All API input validated with `go-playground/validator/v10` via struct tags
- No raw user input in database queries
- Parameterized queries only

## API Key Handling

- Keys are hashed before storage
- Never log API keys
- Use constant-time comparison
- Profile-bound runtime keys separate from operator commands

## Raw Cypher Safeguards

Any operator-only raw Cypher entrypoint must enforce:
- Read-only mode
- Query timeout (30s default)
- Result cap (1000 rows)
- Mandatory `profile_id` filter injection
- No APOC/network/file procedures

## Rate Limiting

- Per-profile rate limits
- Redis counters with TTL
- 429 response when exceeded

## Error Responses

- Never expose stack traces
- Never expose internal IDs
- Never expose database errors
- Generic messages for 500 errors

## Logging

- Log security events (auth failures, rate limits)
- Never log secrets, keys, tokens
- Include profileId in logs for audit
