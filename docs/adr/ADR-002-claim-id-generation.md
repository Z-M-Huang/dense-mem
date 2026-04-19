# ADR-002: Claim ID Generation

**Status**: Accepted  
**Date**: 2026-04-19

## Context

Claim IDs must be:
1. Globally unique to avoid collisions across profiles
2. Deterministic for idempotent writes (same input = same ID)
3. Non-guessable to prevent enumeration attacks

## Decision

Claim IDs are **deterministic UUIDs (UUIDv5)** derived from:
- The profile ID
- The content hash of the claim text (SHA-256, first 32 bytes)
- A fixed namespace UUID for the dense-mem claim namespace

This produces a stable ID for a given (profile, content) pair, enabling
`MERGE` semantics in Neo4j — duplicate claim submissions converge on the same
node rather than creating duplicates.

An idempotency key (supplied by the caller, optional) is also checked before
derivation. If a claim with the same idempotency key already exists for the
profile, the existing claim is returned without a write.

## Consequences

- The same claim text submitted twice for the same profile produces the same
  claim ID and returns the existing claim (HTTP 200, not 201).
- Profile A and profile B can submit identical claim text and receive different
  claim IDs (profile isolation is part of the derivation input).
- UUIDv5 is not random; callers must not use claim IDs as security tokens.
