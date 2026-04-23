# Dense-Mem UAT — Knowledge Pipeline

Playwright end-to-end tests for the dense-mem knowledge pipeline API.

## UAT Coverage Map

| File | UATs | Phase |
|------|------|-------|
| `phase1-indexes.spec.ts` | UAT-01a–e, AC-X2 regression | Neo4j schema bootstrap |
| `phase2-claim-create.spec.ts` | UAT-02a–e | Claim creation & profile isolation |
| `phase2-claim-dedupe.spec.ts` | UAT-03a–d | Claim deduplication |
| `phase3-verify.spec.ts` | UAT-04a–d | Claim verification happy path |
| `phase3-verify-errors.spec.ts` | UAT-05a–d, AC-X6 regression | Verify error taxonomy |
| `phase4-promote.spec.ts` | UAT-06a–f | Promote to fact, list & get facts |
| `phase4-contradiction.spec.ts` | UAT-07a–d | Contradiction detection on promote |
| `phase4-promote-concurrency.spec.ts` | UAT-08a–c | Concurrent promote idempotency |
| `phase6-retract.spec.ts` | UAT-09a–e | Fragment retraction & cascade |
| `phase7-community.spec.ts` | UAT-10a–d | Community detection |
| `phase8-mcp.spec.ts` | UAT-11a–d | MCP server tool surface |
| `phase9-recall.spec.ts` | UAT-12a–e, AC-X2 regression | Semantic recall endpoint |
| `e2e-journey.spec.ts` | UAT-13, AC-X2, AC-X6, isolation | Full pipeline end-to-end |

## Prerequisites

1. A running dense-mem server (default `http://localhost:8080`)
2. Neo4j reachable (default `bolt://localhost:7687`)
3. Valid API key and profile ID

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BASE_URL` | Server base URL | `http://localhost:8080` |
| `API_KEY` | Standard API key | `test-api-key` |
| `PROFILE_ID` | Default profile ID for tests | `test-profile-id` |
| `NEO4J_URI` | Neo4j Bolt URI | `bolt://localhost:7687` |
| `NEO4J_USER` | Neo4j username | `neo4j` |
| `NEO4J_PASSWORD` | Neo4j password | `password` |
| `MCP_BIN` | Path to MCP binary or `main.go` | `./cmd/mcp/main.go` |

## Running Tests

```bash
# Install dependencies
cd tests/uat
npm install
npx playwright install

# List all tests (red-scaffold check — no server required)
npx playwright test --list

# Run all tests against a live server
BASE_URL=http://localhost:8080 API_KEY=<key> PROFILE_ID=<id> \
  npx playwright test

# Run a single phase
npx playwright test phase2-claim-create.spec.ts

# Run with verbose output
npx playwright test --reporter=list
```

## Red/Green Status

All tests in this scaffold are **RED by design** until the corresponding implementation
units are complete. The `--list` command (backpressure gate for Unit 1) passes as soon
as the test files are syntactically valid — actual test execution requires a running server.

## Helper Utilities (`helpers.ts`)

| Export | Purpose |
|--------|---------|
| `headers(profileId)` | Standard auth headers for a profile |
| `neo4jQuery(cypher, params)` | Direct Neo4j query for assertions |
| `spawnMcp(env)` | Spawn and communicate with the MCP binary |
| `seedFragmentForProfile(request, profileId, content, opts)` | Create a fragment via API |
| `createAndVerifyClaim(request, profileId, opts)` | Create + verify a claim |
| `createAndPromoteClaim(request, profileId, opts)` | Create + verify + promote a claim |
| `createTwoSupportPromotedFact(request, profileId, subject)` | Promote two facts for same subject |
| `createValidatedCandidateForVerify(request, profileId, opts)` | Create a validated claim |

## Route Surface Under Test

```
GET  /api/v1/recall
POST /api/v1/claims
GET  /api/v1/claims/:id
POST /api/v1/claims/:id/verify
POST /api/v1/claims/:id/promote
GET  /api/v1/facts/:id
GET  /api/v1/facts
POST /api/v1/fragments
POST /api/v1/fragments/:id/retract
POST /api/v1/tools/detect_community
GET  /api/v1/openapi.json
```
