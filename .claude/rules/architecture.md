# Dense-Mem Architecture

System architecture loaded at session start.

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26 |
| HTTP | `github.com/labstack/echo/v5` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/postgres` |
| Validation | `github.com/go-playground/validator/v10` |
| Neo4j | `github.com/neo4j/neo4j-go-driver/v5` |
| Redis | `github.com/redis/go-redis/v9` |
| Config | `github.com/spf13/viper` or env vars |

## System Overview

```mermaid
flowchart TB
    subgraph Clients["CLIENTS"]
        DL["Digital-Life"]
        CC["Claude Code"]
        OC["OpenClaw"]
    end

    subgraph DenseMem["DENSE-MEM"]
        HTTP["HTTP API"]
        SSE["SSE Streaming"]
        Tools["Agent Tools"]

        Services["Service Layer"]

        Neo4j["Neo4j<br/>Graph + Vectors"]
        Pg["Postgres<br/>Operational"]
        Redis["Redis<br/>Cache"]
    end

    Clients -->|"API Key + X-Profile-ID"| HTTP
    Clients -->|"API Key + X-Profile-ID"| SSE
    Clients -->|"Tool Calls"| Tools

    HTTP --> Services --> Neo4j
    HTTP --> Services --> Pg
    HTTP --> Services --> Redis
```

## Data Stores

| Store | Contents | Isolation Method |
|-------|----------|------------------|
| Neo4j | Knowledge graph, vector indexes | `profile_id` property on every node |
| Postgres | Profiles, API keys, audit logs | `profile_id` column + RLS |
| Redis | Cache, rate limits | Key prefix `profile:{id}:` |

## Knowledge Pipeline

```mermaid
flowchart LR
    Raw["Raw Evidence"] -->|"POST /fragments"| SF["SourceFragment"]
    SF -->|"LLM extract"| Claim["Claim (candidate)"]
    Claim -->|"verify"| VClaim["Claim (validated)"]
    VClaim -->|"promote"| Fact["Fact"]

    SF -.->|"SUPPORTED_BY"| Claim
    Claim -.->|"PROMOTES_TO"| Fact
```

## Authentication

| Key Type | Purpose | Endpoints |
|----------|---------|-----------|
| Standard API Key | Regular operations | `/profiles`, `/knowledge`, `/search`, `/tools` |
| Admin API Key | Admin operations | `/admin/graph/query` (read-only Cypher) |

## Request Flow

1. Validate API key
2. Extract `X-Profile-ID`
3. Rate limit check (Redis)
4. Input validation (struct validator)
5. Service call with profileId
6. Database queries filter by profile_id
7. Response (JSON or SSE)