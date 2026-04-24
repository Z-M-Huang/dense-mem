# Discovery: extracting digital-life into Dense-Mem

## Scope

This discovery is based on the architecture documentation in `/app/digital-life.wiki`.

Important constraint:
- `/app/digital-life.wiki` is a documentation repo, not an implementation repo.
- `/app/digital-life` currently contains no application source code to inspect.
- So this document captures the **documented architecture, contracts, schemas, and extraction implications**, not audited runtime code.

## Executive summary

`digital-life` is designed as a **single-persona-per-instance** GraphRAG system with:
- **Neo4j** for the knowledge graph and vector/full-text retrieval
- **Postgres** for operational state
- **Redis + BullMQ** for async ingestion and maintenance orchestration
- **Hono + SSE** for API delivery
- **Agent tools** layered over read/write graph operations

For `dense-mem`, the main extraction work is **not** the knowledge model itself. The hard part is converting a system that assumes **one persona per container** into a **strict multi-profile service** where every graph query, SQL query, Redis key, queue, auth check, and stream is profile-scoped.

The best reusable parts are:
- the **three-tier knowledge model** (`SourceFragment -> Claim -> Fact`)
- the **promotion pipeline** and temporal model
- the **hybrid retrieval model** (graph + BM25 + vectors)
- the **tool contracts**
- the **REST + SSE API shape**
- the **polyglot persistence split**

The biggest extraction challenges are:
- multi-tenant isolation in Neo4j
- replacing instance-level trust with request-level authorization
- deciding whether async ingestion still needs a queue layer
- deciding whether `dense-mem` owns extraction/verification/embedding generation or only stores/query memory
- introducing profile-aware auth, API keys, and audit trails that do not exist in the original single-persona design

---

## 1. Current digital-life architecture

## 1.1 Storage responsibilities

### Neo4j
Used for:
- `SourceFragment`, `Claim`, `Fact`, `Person`, `Organization`, `Project`, `Topic`, `Community`, `Gap`, tool-related nodes
- graph traversal
- full-text retrieval
- vector retrieval
- temporal fact versioning and supersession chains

### Postgres
Used for operational data, including:
- sync state
- audit logs
- user/persona config
- credentials / token metadata
- connector state
- potentially job queue state references

### Redis
Used as the async execution backbone in the documented architecture:
- BullMQ job queue
- retries / backpressure / DLQ
- connector -> ingestion pipeline

Note on `dense-mem` deployment: Redis is optional for single-node deployments
and required for multi-instance setups (where rate-limit counters and SSE
concurrency must be shared across replicas).

## 1.2 Layered architecture pattern

Documented top-level flow:

`Sources -> Connectors -> Pre-extraction -> Redis/BullMQ -> Router Agent -> Ingestion/Query/Maintenance Agents -> Neo4j/Postgres -> Hono API -> Frontend/External clients`

Reusable pattern for `dense-mem`:
- **API routes** should stay thin
- **services** should own business logic
- **database clients** stay low-level
- **read/write graph access** should remain explicitly separated

This matches the `dense-mem` CLAUDE.md guidance:

`API Routes -> Services -> Database Clients`

---

## 2. Knowledge pipeline: SourceFragment -> Claim -> Fact

## 2.1 End-to-end flow

The documented ingestion pipeline is:

1. Connector polls external source incrementally using a cursor
2. Connector emits raw events via `AsyncGenerator`
3. Pre-extraction adds deterministic structure
   - Jira keys
   - GitHub refs
   - emails
   - mentions
4. Event is queued in Redis/BullMQ
5. Ingestion Agent dequeues a batch
6. Claim extraction creates candidate `Claim` nodes from source evidence (via `POST /api/v1/claims`)
7. Separate verifier model checks entailment
8. Modality gates block non-factual material
9. Contradiction detection compares against active facts
10. Predicate-specific promotion gates decide whether the claim becomes a fact
11. Neo4j writes `SourceFragment`, `Claim`, and maybe `Fact`
12. Postgres writes sync cursor and audit log entries

## 2.2 SourceFragment

`SourceFragment` is the immutable evidence layer.

### Role
- exact source-backed evidence
- never modified after creation
- foundation for verification and citations

### Key properties
- `fragment_id`: content-derived hash of `connector + source_id + span`
- `connector`
- `source_id`
- `author`
- `content`
- `timestamp`
- `thread_id?`
- `is_quoted`
- `is_forwarded`
- `source_quality`
- `embedding` (1536d)
- `embedding_model`
- `classification` (`public`, `internal`, `confidential`, `restricted`)

### Extraction implication for dense-mem
Keep this node almost unchanged. The main addition is:
- `profile_id`

## 2.3 Claim

`Claim` is the extracted assertion layer.

### Role
- machine-extracted statement from one or more source spans
- may remain tentative forever
- is the unit that passes through verification and promotion gates

### Key properties
- `claim_id`: content-derived hash of `subject + predicate + object + valid_from`
- `predicate`
- `modality`: `assertion`, `question`, `proposal`, `speculation`, `quoted`
- `polarity`
- `speaker`
- `span_start`, `span_end`
- `valid_from`, `valid_to`
- `extract_conf`
- `resolution_conf`
- `entailment_verdict`: `entailed`, `contradicted`, `insufficient`
- `status`: `candidate`, `validated`, `promoted`, `rejected`, `disputed`
- `extraction_model`, `extraction_version`, `verifier_model`
- `pipeline_run_id`

### Important intermediate state: provisional claims
A `validated` claim:
- passed NLI verification
- did **not** pass promotion gates
- is allowed in retrieval as **Tier 1.5**
- must be surfaced with uncertainty language

This is a strong reusable pattern for `dense-mem` because it avoids forcing every assertion into a binary true/false state.

## 2.4 Fact

`Fact` is the canonical promoted truth layer.

### Role
- promoted, versioned, retrievable truth
- used for primary query grounding
- supports freshness and historical querying

### Key properties
- `fact_id`: UUID
- `predicate`
- `status`: `active`, `superseded`, `disputed`, `retracted`, `needs_revalidation`
- `truth_score`
- `valid_from`, `valid_to`
- `recorded_at`, `recorded_to`
- `retracted_at`
- `last_confirmed_at`

## 2.5 Bi-temporal model

Every fact has two time dimensions:

### Valid time
Answers:
- "When was this true in reality?"

Fields:
- `valid_from`
- `valid_to`

### Transaction time
Answers:
- "When did the system know this?"

Fields:
- `recorded_at`
- `recorded_to`

### Why this matters for dense-mem
This is one of the highest-value parts to preserve.
It enables:
- current-state queries
- historical truth queries
- historical system-belief queries
- corrections without losing auditability

## 2.6 Pipeline relationships

### Core edges
- `SUPPORTED_BY`: `Claim -> SourceFragment`
- `PROMOTES_TO`: `Claim -> Fact`
- `SUPERSEDED_BY`: `Fact -> Fact`
- `CONTRADICTS`: `Claim -> Claim/Fact`
- `DERIVED_FROM`: `Claim -> Claim`
- `SUBJECT`: `Claim/Fact -> Entity`
- `OBJECT`: `Claim/Fact -> Entity`
- `CURRENT_*`: derived materialized edges between entities

### Extraction implication
For multi-profile isolation, the safest design is:
- all nodes carry `profile_id`
- all relationships either:
  - connect only same-profile nodes, and/or
  - also carry `profile_id` for defense in depth

The original docs only discuss a single database with no tenant predicate. `dense-mem` should add explicit relationship isolation rules.

## 2.7 Promotion pipeline in detail

Documented stages:

### 1. Claim extraction
Creates candidate claims from a source fragment (caller-initiated via `POST /api/v1/claims`).

### 2. Verifier pass
A separate model sees:
- exact source span
- extracted claim

Outputs one of:
- `entailed`
- `contradicted`
- `insufficient`

Only `entailed` claims continue.

### 3. Modality gates
These **never auto-promote**:
- `question`
- `proposal`
- `speculation`
- `quoted`
- ambiguous time expressions

Only `assertion` with clear temporal context proceeds.

### 4. Contradiction detection
Before promotion, compare the verified claim against active facts for the same subject + predicate.

Outcomes:
- same object -> confirm existing fact
- stronger conflicting claim -> supersede old fact
- comparable conflict -> mark disputed + create `CONTRADICTS`
- weaker claim -> reject
- multi-valued predicate -> coexist

### 5. Predicate-specific promotion gates
Policies are defined per predicate:
- `single_current`
- `multi_valued`
- `versioned`
- `append_only`

For high-risk `single_current` predicates, all hard gates must pass:
- `extract_conf >= threshold`
- `resolution_conf >= threshold`
- `entailment_verdict = 'entailed'`
- `modality = 'assertion'`
- `(source_count >= 2 OR max_source_quality >= 0.95)`

### 6. Write model
Outcomes:
- pass all gates -> create `Fact` with `status = active`
- verified but insufficient -> keep as `validated` claim
- failed verification / insufficient evidence -> keep candidate or reject

## 2.8 Supersession and deletion model

This is especially important for a memory service.

### Supersession
When a better fact arrives:
- old fact becomes `superseded`
- old fact gets `recorded_to`
- new fact is created
- `SUPERSEDED_BY` edge connects them

### No hard deletes
If evidence disappears:
1. mark `SourceFragment` unavailable
2. recompute fact support
3. if support is gone, mark fact `needs_revalidation`
4. do not delete fact history

### Reuse recommendation
Preserve this exactly in `dense-mem`.
It is the right model for long-lived memory systems.

---

## 3. Retrieval model that sits on top of the pipeline

Even though the request focuses on ingestion, the pipeline only makes sense with retrieval.

## 3.1 Tiered retrieval

The Query Agent uses four retrieval tiers:

- **Tier 1**: promoted `Fact`
- **Tier 1.5**: `validated` claims
- **Tier 2**: candidate claims
- **Tier 3**: semantic backstop search

This is a reusable retrieval architecture for `dense-mem` because it separates:
- canonical truth
- plausible but unconfirmed memory
- weak recall backstops

## 3.2 Hybrid retrieval modes

Each tier combines three modes:
- **BM25 keyword**
- **vector similarity**
- **graph traversal**

Merged with **Reciprocal Rank Fusion (RRF)**.

### Indexes documented in the wiki
- full-text on `SourceFragment.content`
- full-text on `Fact.predicate`
- vector index on `SourceFragment.embedding`
- vector index on `Topic.embedding`

### Reuse recommendation
This hybrid retrieval shape is worth carrying over directly.
It maps cleanly to:
- `graph-query`
- `keyword-search`
- `semantic-search`

---

## 4. Profile isolation mechanisms

## 4.1 What exists today in digital-life

`digital-life` is explicitly designed as:
- one Docker container
- one persona
- one Neo4j database
- one Postgres operational store
- one Redis namespace

Documented implications:
- Neo4j: no multi-tenant complexity
- Postgres: no `persona_id` columns needed
- Redis: single queue namespace
- API: no profile switching or profile ownership validation
- `graph-query` explicitly says no isolation predicate is needed

## 4.2 Signals that profile-awareness was anticipated

A few docs hint at future multi-profile concerns:
- connector tokens use **per-profile key derivation**
- maintenance graph projection mentions **filter by profile + signal strength**
- tool-learning defines a kill switch reason: `cross_profile_isolation_violation`
- tool permissions include `graphWriteNamespace` reserved for future use

These are not a full multi-tenant design, but they show the intended direction.

## 4.3 What dense-mem must add

### Neo4j isolation
Dense-mem README already proposes:
- `profile_id` property on every node
- filtered queries

That is necessary but not sufficient.

Recommended hardening:
1. Put `profile_id` on **every node**
2. Prefer also putting `profile_id` on **every relationship**
3. All service-layer queries must anchor on `profile_id`
4. Tool-level graph access must inject profile filtering automatically
5. Writes must validate that both ends of a relationship belong to the same profile
6. Add invariant scans for cross-profile edges

### Postgres isolation
Dense-mem README proposes:
- `profile_id` column with RLS policies

Recommended operational tables to scope by `profile_id`:
- profiles
- api_keys
- connector_credentials
- connector_sync_state
- audit_log
- sessions
- caches / materialized summaries metadata

### Redis isolation
Dense-mem README proposes key prefixing:
- `profile:{id}:...`

Recommended prefixes:
- `profile:{id}:ratelimit:{key}`
- `profile:{id}:cache:{queryHash}`
- `profile:{id}:session:{sessionId}`
- `profile:{id}:stream:{requestId}`
- `profile:{id}:jobs:{queueName}` if async jobs remain

## 4.4 Major extraction challenge

The current system assumes trust comes from **deployment isolation**.
Dense-mem must move that trust boundary into **application logic**.

That changes everything:
- auth model
- route model
- query generation
- DB schema
- rate limiting
- SSE stream ownership
- audit trails
- testing strategy

---

## 5. Agent tools and implementation patterns

Important note:
- The wiki defines **tool contracts and responsibilities**.
- No concrete tool source files were found in the repo.
- So the items below are the documented implementation patterns to adapt, not audited code.

## 5.1 Common tool pattern

Documented pattern:
- Vercel AI SDK `tool()`
- Zod input schemas
- `execute` function performs the operation
- tools are registered per-agent
- query tools use read-only Neo4j access

Likely original package location from docs:
- `packages/agents/src/tools/{name}.ts`

For `dense-mem`, the same logical tools can exist in two forms:
1. **internal service tools** used by agents
2. **HTTP tool endpoints** for external callers

## 5.2 graph-query

### Documented contract
Parameters:
- `query: string`
- `params?: Record<string, unknown>`

Guarantees:
- read-only Neo4j session
- `executeRead()` only
- parameterized Cypher
- Cypher comes from the agent, not raw user input
- returns row objects

### Current digital-life assumption
- one container = one database
- therefore no isolation predicate required

### Dense-mem adaptation
This is the most important tool to redesign.

Recommended implementation shape:
- accept `profileId`
- reject queries lacking a profile scope unless explicitly operator-run
- inject or enforce profile predicates
- allow only read-only clauses
- optionally parse/guard query shape to block writes, procedures, schema ops

### Service signature suggestion
```ts
interface GraphQueryInput {
  profileId: string;
  query: string;
  params?: Record<string, unknown>;
}
```

### Practical challenge
LLM-generated Cypher is dangerous in a multi-tenant graph.
In `digital-life`, read-only session was enough.
In `dense-mem`, you also need **tenant-safe query shaping**.

## 5.3 semantic-search

### Documented purpose
- vector similarity within a scoped graph neighborhood
- used by Query and Ingestion agents
- backed by Neo4j vector indexes

### Documented indexes
- `SourceFragment.embedding` (1536d, cosine)
- `Topic.embedding` (1536d, cosine)

### Likely implementation pattern to adapt
1. embed the query text
2. query Neo4j vector index
3. optionally constrain by:
   - entity anchors
   - neighborhood
   - time window
   - predicate / node labels
4. return ranked evidence candidates

### Dense-mem adaptation
Decide early whether `dense-mem`:
- generates embeddings itself, or
- accepts embeddings supplied by clients

This is a real gap between the docs and the target stack.
The target stack lists:
- Bun
- Hono
- Zod
- neo4j-driver
- postgres.js
- ioredis

It does **not** define an embedding provider.
So semantic search needs one of:
- built-in embedding generation
- pluggable embedding provider interface
- client-supplied vectors

## 5.4 keyword-search

### Documented purpose
- BM25 full-text search via Neo4j full-text index
- used by Query Agent

### Documented indexes
- full-text on `SourceFragment.content`
- full-text on `Fact.predicate`

### Likely implementation pattern to adapt
- run full-text search against source fragments and possibly fact/predicate surfaces
- merge results with vector and graph retrieval
- rerank based on query intent

### Dense-mem adaptation
Recommended input:
```ts
interface KeywordSearchInput {
  profileId: string;
  query: string;
  limit?: number;
  labels?: string[];
  validAt?: string;
  knownAt?: string;
}
```

Important multi-tenant issue:
- Neo4j full-text indexes are global to the database
- so result filtering by `profile_id` is mandatory after search unless the indexed content itself is profile-scoped at query time

## 5.5 community-summary

### Documented purpose
- retrieve precomputed Leiden community summaries
- used by Query Agent for "big picture" context

### Backing data model
`Community` node properties include:
- `community_id`
- `level`
- `summary`
- `summary_version`
- `member_count`
- `top_entities[]`
- `last_summarized_at`

Related edges:
- `IN_COMMUNITY` from entities to communities

### Documented maintenance pipeline
- build graph projection from active facts
- filter by profile + signal strength
- run Leiden via Neo4j GDS
- create L0/L1/L2 communities
- summarize bottom-up
- only re-summarize communities whose membership changed

### Dense-mem adaptation
This tool is reusable, but it depends on a non-trivial maintenance subsystem:
- GDS availability
- projection strategy
- summarization job
- versioned community nodes

This may be a **Phase 2 feature** for `dense-mem`, not MVP.

## 5.6 Tool transparency and streaming

The API docs define SSE events including:
- `tool_call`
- `text_delta`
- `evidence`
- `gap_detected`
- `done`
- `error`

This is a useful reusable pattern for `dense-mem` when exposing agent tools over SSE.

---

## 6. API patterns and authentication model

## 6.1 Current digital-life API pattern

Documented request flow:

`Client -> Auth Middleware -> Data Classification Middleware -> Hono Router -> REST or SSE Handler -> Router Agent -> Execution Agent`

This layering is worth reusing directly.

## 6.2 Auth model in digital-life

### Browser/frontend
- session cookie
- HTTP-only
- secure
- SameSite=Strict

### External API
- API key in `Authorization: Bearer <key>`

### Important limitation
Because `digital-life` is single persona per instance:
- one instance-wide operator credential can grant access to the whole instance
- there is no profile ownership model to inspect

So there is **no existing multi-profile authorization implementation to extract**.

## 6.3 API shape in digital-life

### REST
Management-style endpoints for:
- persona
- bootstrap
- connectors
- gaps
- knowledge stats/entities/facts/communities
- health and sync status

### SSE
Primary query endpoint:
- `POST /api/query`
- `Accept: text/event-stream`

Payload supports:
- `message`
- `conversationId`
- `options.includeEvidence`
- `options.queryType`
- `options.validAt`
- `options.knownAt`

### SSE events
- `text_delta`
- `tool_call`
- `evidence`
- `gap_detected`
- `done`
- `error`

## 6.4 Error envelope

Documented shape:
```ts
interface ApiError {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}
```

Reusable for `dense-mem` as-is.

## 6.5 Rate limiting

Documented patterns:
- frontend: sliding window
- external API key: token bucket
- streaming queries: semaphore / concurrent stream cap

Dense-mem can reuse the strategy, but should make limits profile-aware and key-aware.

## 6.6 Data classification middleware

Current digital-life includes classification middleware that:
1. authenticates inbound request
2. filters outbound sensitivity by caller access level
3. may route sensitive workloads to local LLMs

For `dense-mem`, classification may or may not remain in scope.
If this service is purely a memory backend, classification can be preserved as metadata and policy hooks, but full LLM-routing policy may belong to the caller.

## 6.7 Dense-mem API recommendation

A good extraction pattern is:

### Middleware chain
`request -> auth -> profile resolution -> profile authorization -> rate limit -> zod validation -> service -> stream/response`

### Route styles
Use both:
- path-scoped routes: `/api/v1/profiles/:id/...` for profile-management APIs
- auth-scoped tool/data routes that derive profile from the profile-bound API key

### Why prefer path-scoped routes
They are safer because:
- easier to audit in logs
- harder to omit accidentally
- easier to reason about in Hono middleware

Recommended exception:
- legacy clients may send `X-Profile-ID`, but current clients should rely on the API key profile binding

---

## 7. Database schema details extracted from the wiki

## 7.1 Neo4j node schemas

### SourceFragment
Properties:
- `fragment_id`
- `connector`
- `source_id`
- `author`
- `content`
- `timestamp`
- `thread_id?`
- `is_quoted`
- `is_forwarded`
- `source_quality`
- `embedding`
- `embedding_model`
- `classification`

### Claim
Properties:
- `claim_id`
- `predicate`
- `modality`
- `polarity`
- `speaker`
- `span_start`
- `span_end`
- `valid_from`
- `valid_to`
- `extract_conf`
- `resolution_conf`
- `entailment_verdict`
- `status`
- `extraction_model`
- `extraction_version`
- `verifier_model`
- `pipeline_run_id`

### Fact
Properties:
- `fact_id`
- `predicate`
- `status`
- `truth_score`
- `valid_from`
- `valid_to`
- `recorded_at`
- `recorded_to`
- `retracted_at`
- `last_confirmed_at`

### Entity and system nodes
- `Person`
- `Organization`
- `Project`
- `Topic`
- `Community`
- `Gap`
- `ToolNeed`
- `PendingTool`
- `ActiveTool`
- `Capability`

### Dense-mem recommendation
All of the above should gain:
- `profile_id`

## 7.2 Neo4j indexes

Documented indexes:
- lookup: `SourceFragment.fragment_id`
- lookup: `SourceFragment.source_id`
- lookup: `Claim.claim_id`
- lookup: `Fact.fact_id`
- lookup: `Fact.status`
- lookup: `Person.email`
- lookup: `Topic.name`
- composite: `SourceFragment.connector`, `SourceFragment.timestamp`
- composite: `Fact.predicate`, `Fact.status`
- full-text: `SourceFragment.content`
- full-text: `Fact.predicate`
- vector: `SourceFragment.embedding`
- vector: `Topic.embedding`

### Dense-mem recommendation
Add tenant-aware indexes, likely:
- `(profile_id, fragment_id)` or unique `fragment_id` if globally content-derived
- `(profile_id, claim_id)`
- `(profile_id, fact_id)`
- `(profile_id, status)`
- `(profile_id, predicate, status)`
- `(profile_id, connector, timestamp)`

If Neo4j constraints cannot express every desired composite uniqueness pattern cleanly, keep globally unique IDs and still index `profile_id` heavily.

## 7.3 Neo4j constraints

Documented constraints:
- unique `SourceFragment.fragment_id`
- unique `Claim.claim_id`
- unique `Fact.fact_id`
- unique `Person.person_id`

### Multi-profile consideration
If deterministic IDs are only unique within a profile, switch to composite uniqueness.
If IDs remain globally unique, preserve current uniqueness and add secondary profile indexes.

## 7.4 Postgres operational schema details available in the docs

The wiki does **not** define full SQL DDL, but it does specify several operational records.

### Connector sync state
Documented fields:
- `connectorId: string`
- `lastSyncAt: Date`
- `cursor: string | null`
- `status: 'idle' | 'syncing' | 'error'`
- `errorMessage?: string`
- `itemsSynced: number`

### Audit log
Documented fields:
- `timestamp: Date`
- `operation: enum('create'|'update'|'merge'|'supersede'|'retract')`
- `entityType: string`
- `entityId: string`
- `before: object | null`
- `after: object`
- `reason: string`

Documented storage properties:
- append-only
- partitioned by month
- stored in Postgres, not Neo4j

### Credentials / token storage
Documented behavior:
- tokens encrypted at rest
- AES-256-GCM mentioned in bootstrap docs
- key derived from `SECRETS_ENCRYPTION_KEY`
- stored in Postgres, not Neo4j
- per-profile key derivation referenced in connector docs
- tokens never logged or returned

### Gaps in the docs
The wiki does **not** provide concrete schemas for:
- API keys
- sessions
- profiles/personas in a multi-tenant service
- connector credential table columns
- query cache tables
- SSE session persistence

These must be designed fresh for `dense-mem`.

## 7.5 Redis patterns

Documented digital-life usage:
- BullMQ queue backing store
- retries
- backpressure
- DLQ

Dense-mem README currently says Redis is for:
- query cache
- rate limiting
- session state

### Extraction concern
If `dense-mem` intends to keep async verification/promotion, Redis alone is not enough as a concept; you still need:
- BullMQ or equivalent queueing
- retry semantics
- dead-letter handling
- job ownership per profile

---

## 8. Reusable architecture patterns

## 8.1 Highly reusable

### Three-tier knowledge model
Strong fit for standalone memory service.

### Bi-temporal facts
Should be preserved exactly.

### Explicit promotion pipeline
Prevents low-quality memory pollution.

### Read/write Neo4j separation
Keep this invariant.

### Polyglot persistence
Still correct for `dense-mem`.

### Hybrid retrieval
BM25 + vector + graph is a strong pattern.

### REST + SSE split
Good fit for both direct API clients and agent runtimes.

### Append-only audit log
Essential in a multi-profile service.

## 8.2 Reusable with modification

### Agent tools
Need tenant-safe wrappers.

### Router/agent architecture
Useful if `dense-mem` owns ingestion and query reasoning.
Potentially overkill if the service is only a memory CRUD/retrieval backend.

### Community summaries
Useful, but depends on maintenance/GDS pipeline.

### Data classification middleware
Useful if `dense-mem` will run LLM workloads internally.
Less central if callers do their own model orchestration.

---

## 9. Existing implementations to adapt

Because there is no runtime code in the repo, "existing implementations" means **documented contracts and package patterns**.

## 9.1 Best concrete things to adapt

### Tool contracts
Especially:
- `graph-query`
- `semantic-search`
- `keyword-search`
- `community-summary`

### Package boundaries
From the documented monorepo:
- `core`
- `connectors`
- `agents`
- `orchestrator`
- `api`

For `dense-mem`, a simplified version could be:
- `core`
- `storage`
- `services`
- `api`
- `tools`

### API streaming event model
Reuse:
- `tool_call`
- `text_delta`
- `evidence`
- `done`
- `error`

### Neo4j schema and indexes
These are the most directly portable assets from the wiki.

### Operational table concepts
Reuse:
- sync state
- audit log
- credential storage patterns

---

## 10. Potential challenges for extraction

## 10.1 No source code exists to lift directly

This is the biggest practical reality.
You are extracting from a **design spec**, not a running implementation.

Implication:
- expect greenfield implementation work
- treat the wiki as product architecture, not code truth

## 10.2 Multi-tenant Neo4j safety

The original system avoids tenant complexity entirely.
`dense-mem` must solve:
- profile filters on every query
- relationship leakage
- full-text and vector result filtering
- tenant-aware maintenance jobs
- tenant-aware invariants

## 10.3 LLM-generated Cypher in a multi-profile graph

`graph-query` becomes riskier when a missed predicate means cross-profile exposure.

Mitigations:
- allowlisted read-only query shapes
- query builder templates for common retrievals
- profile predicate injection
- operator-only raw Cypher endpoint

## 10.4 Queueing gap

`digital-life` depends on BullMQ for:
- verifier retries
- async promotion
- pipeline backpressure
- failure recovery

`dense-mem` target stack lists `ioredis`, but not BullMQ.
So decide whether to:
- add BullMQ, or
- implement a simpler async job layer, or
- make ingestion synchronous and accept the tradeoffs

## 10.5 Embedding ownership gap

The semantic-search design assumes embeddings exist.
The target stack does not specify who creates them.

Need a choice:
- `dense-mem` generates embeddings
- caller generates embeddings
- both, via pluggable provider interface

## 10.6 Auth model redesign

Original model:
- one operator credential
- one persona

Dense-mem needs:
- multiple API keys
- profile scoping
- maybe per-key permissions
- maybe operator vs runtime scopes
- maybe session support for browser clients

## 10.7 Community summaries may be too large for MVP

They depend on:
- GDS / Leiden
- summarization jobs
- maintenance scheduling
- change detection
- summary versioning

This is extractable, but likely not day-one critical.

## 10.8 Operational schema is under-specified

Neo4j is well specified.
Postgres is only partially specified.
You must design from scratch:
- `profiles`
- `api_keys`
- `sessions`
- `connector_credentials`
- `connector_sync_state`
- `audit_log`
- maybe `jobs`
- maybe `stream_sessions`

---

## 11. Recommended dense-mem extraction plan

## 11.1 Phase 1: foundation

Implement first:
- profile model
- API key model
- auth middleware
- profile authorization middleware
- Neo4j client with enforced `profile_id`
- Postgres schema for profiles, keys, audit log
- Redis rate limiting and cache prefixing
- basic `graph-query`, `keyword-search`, `semantic-search`
- SSE response envelope

## 11.2 Phase 2: pipeline

Add:
- `SourceFragment`, `Claim`, `Fact` writes
- promotion statuses
- supersession model
- evidence chain retrieval
- optional async verification/promotion queue

## 11.3 Phase 3: advanced retrieval

Add:
- tiered retrieval
- RRF hybrid ranking
- temporal query modes
- evidence-rich SSE query responses

## 11.4 Phase 4: maintenance/intelligence

Add later:
- revalidation queue
- invariant scans
- community detection
- community summarization
- reflection / contradiction workflows

---

## 12. Concrete recommendations for dense-mem

## 12.1 Preserve unchanged
- `SourceFragment -> Claim -> Fact`
- `SUPPORTED_BY`, `PROMOTES_TO`, `SUPERSEDED_BY`, `CONTRADICTS`
- bi-temporal fact fields
- append-only audit logging
- read vs write graph sessions
- REST + SSE split

## 12.2 Change immediately for multi-profile
- add `profile_id` everywhere
- redesign auth for key-to-profile authorization
- make all middleware profile-aware
- make all Redis keys profile-prefixed
- add SQL RLS or equivalent hard guards
- add graph invariants for cross-profile edges

## 12.3 Defer until later
- full Router/agent orchestration
- GDS/Leiden communities
- self-reflection pipeline
- learned tools / autonomous growth

---

## 13. Bottom line

The wiki describes a strong memory architecture, but it is optimized for **single-persona deployment isolation**, not app-level multi-tenancy.

For `dense-mem`, the core extraction should be:
1. keep the **knowledge model**
2. keep the **promotion and temporal semantics**
3. keep the **tool and API patterns**
4. redesign **all isolation and auth boundaries** around `profile_id`

If done well, `dense-mem` becomes the reusable multi-profile memory substrate that `digital-life` originally avoided building by using one-container-per-persona deployment.
