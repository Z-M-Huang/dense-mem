# syntax=docker/dockerfile:1.7

# ============================================================================
# Build stage
# ============================================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Module download in its own layer so source edits do not bust the cache.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# CGO=0 produces a static binary that runs on alpine without libc shims.
# -trimpath strips build-host paths; -ldflags="-s -w" drops symbol + DWARF
# tables (smaller image, no debugger support in prod).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ============================================================================
# Runtime stage
# ============================================================================
FROM alpine:3.20

# ca-certificates for outbound TLS (Postgres/Neo4j/Redis if TLS-enabled);
# tzdata for correct UTC handling in audit timestamps; wget for HEALTHCHECK.
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S densemem && \
    adduser -S -G densemem -H -s /sbin/nologin densemem

WORKDIR /app

COPY --from=builder /out/server  /app/server
COPY --from=builder /out/migrate /app/migrate

# migrator.go discovers migrations via cwd-relative walk; WORKDIR=/app plus
# this copy satisfies Strategy 1 in getMigrationsDir().
COPY --chown=densemem:densemem migrations /app/migrations

# Entrypoint wrapper assembles POSTGRES_DSN from component env vars if the
# DSN is not supplied directly. Keeps the full credentialed URL literal out
# of every tracked config file.
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

USER densemem

EXPOSE 8080

# /health is a liveness probe (process up); /ready flips to 503 on transient
# dependency blips which would force Docker to restart a healthy container.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget --quiet --spider http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/server"]
