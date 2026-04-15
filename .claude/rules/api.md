---
paths:
  - "internal/http/**/*.go"
  - "cmd/server/**/*.go"
---

# API Design Rules

Rules for API routes and handlers.

## Endpoint Structure

- `/api/v1/profiles/:id/...` for profile-scoped operations
- `/api/v1/tools/...` for agent tools
- `/admin/...` for admin endpoints (requires admin key)

## Middleware Chain

1. Auth middleware: validate API key
2. Profile middleware: extract and validate `X-Profile-ID`
3. Rate limit: per-profile check (Redis)
4. Validation: struct tags via `go-playground/validator/v10`

## Response Format

Success: `{ "data": T }`
Error: `{ "error": string, "code"?: string }`

Use appropriate HTTP status codes:
- 200: Success
- 201: Created
- 400: Validation error
- 404: Not found
- 500: Internal error

## SSE Streaming

For long operations:
- Set `Content-Type: text/event-stream`
- Stream results as they arrive
- Handle client disconnect via `context.Context` cancellation

## Input Validation

All endpoints validate with `go-playground/validator/v10`. Define request DTOs
with struct tags in `internal/http/dto/`, wire validator via Echo binding.

Never trust raw input. Always validate before processing.
