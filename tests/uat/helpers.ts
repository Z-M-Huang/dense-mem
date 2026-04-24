import { APIRequestContext, expect } from '@playwright/test';
import neo4j, { Driver, Session } from 'neo4j-driver';
import { ChildProcess, spawn } from 'child_process';

// ---------------------------------------------------------------------------
// Environment helpers
// ---------------------------------------------------------------------------

export const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
export const API_KEY = process.env.API_KEY || 'test-api-key';
export const PROFILE_ID = process.env.PROFILE_ID || 'test-profile-id';
export const DENSE_MEM_URL = process.env.DENSE_MEM_URL || BASE_URL;
export const DENSE_MEM_API_KEY = process.env.DENSE_MEM_API_KEY || API_KEY;

const NEO4J_URI = process.env.NEO4J_URI || 'bolt://localhost:7687';
const NEO4J_USER = process.env.NEO4J_USER || 'neo4j';
const NEO4J_PASSWORD = process.env.NEO4J_PASSWORD || 'password';

// ---------------------------------------------------------------------------
// HTTP header factories
// ---------------------------------------------------------------------------

/** Standard authenticated headers. Profile scope is derived from the API key. */
export function headers(_profileId?: string): Record<string, string> {
  return {
    'Authorization': `Bearer ${API_KEY}`,
    'Content-Type': 'application/json',
  };
}

// ---------------------------------------------------------------------------
// Neo4j direct query helper
// ---------------------------------------------------------------------------

let _driver: Driver | null = null;

function getNeo4jDriver(): Driver {
  if (!_driver) {
    _driver = neo4j.driver(
      NEO4J_URI,
      neo4j.auth.basic(NEO4J_USER, NEO4J_PASSWORD),
    );
  }
  return _driver;
}

/**
 * Execute a Cypher query directly against Neo4j.
 * Returns an array of record objects (key → native JS value).
 */
export async function neo4jQuery(
  cypher: string,
  params: Record<string, unknown> = {},
): Promise<Record<string, unknown>[]> {
  const driver = getNeo4jDriver();
  const session: Session = driver.session({ database: 'neo4j' });
  try {
    const result = await session.run(cypher, params);
    return result.records.map((r) => {
      const obj: Record<string, unknown> = {};
      for (const key of r.keys) {
        obj[key as string] = r.get(key as string);
      }
      return obj;
    });
  } finally {
    await session.close();
  }
}

/** Close the shared Neo4j driver — call in globalTeardown if needed. */
export async function closeNeo4j(): Promise<void> {
  if (_driver) {
    await _driver.close();
    _driver = null;
  }
}

// ---------------------------------------------------------------------------
// MCP server helper
// ---------------------------------------------------------------------------

export interface McpHandle {
  proc: ChildProcess;
  stdin: NodeJS.WritableStream;
  /** Send a JSON-RPC request and wait for the first response line. */
  call(method: string, params?: unknown): Promise<unknown>;
  /** Terminate the MCP server process. */
  close(): Promise<void>;
}

/**
 * Spawn the dense-mem MCP server binary for MCP UAT tests.
 * The MCP binary is an HTTP-backed facade and requires DENSE_MEM_URL and
 * DENSE_MEM_API_KEY.
 */
export async function spawnMcp(
  env: Record<string, string> = {},
): Promise<McpHandle> {
  const mcpBin = process.env.MCP_BIN || './cmd/mcp/main.go';
  const proc = spawn('go', ['run', mcpBin], {
    env: { ...process.env, ...env },
    stdio: ['pipe', 'pipe', 'pipe'],
  });

  const lines: string[] = [];
  let resolver: ((line: string) => void) | null = null;

  proc.stdout?.on('data', (chunk: Buffer) => {
    const text = chunk.toString();
    for (const line of text.split('\n')) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      if (resolver) {
        resolver(trimmed);
        resolver = null;
      } else {
        lines.push(trimmed);
      }
    }
  });

  // Wait briefly for the process to start
  await new Promise<void>((res) => setTimeout(res, 500));

  const handle: McpHandle = {
    proc,
    stdin: proc.stdin!,
    call(method: string, params?: unknown): Promise<unknown> {
      return new Promise((resolve, reject) => {
        const msg = JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }) + '\n';
        if (lines.length > 0) {
          const line = lines.shift()!;
          try { resolve(JSON.parse(line)); } catch { reject(new Error(`Invalid JSON: ${line}`)); }
        } else {
          resolver = (line: string) => {
            try { resolve(JSON.parse(line)); } catch { reject(new Error(`Invalid JSON: ${line}`)); }
          };
        }
        proc.stdin!.write(msg);
      });
    },
    close(): Promise<void> {
      return new Promise((res) => {
        proc.kill();
        proc.on('exit', () => res());
        setTimeout(() => { proc.kill('SIGKILL'); res(); }, 3000);
      });
    },
  };

  return handle;
}

// ---------------------------------------------------------------------------
// Seeding helpers
// ---------------------------------------------------------------------------

/**
 * Create a fragment for the given profile via POST /api/v1/fragments.
 * Returns the parsed response body.
 */
export async function seedFragmentForProfile(
  request: APIRequestContext,
  profileId: string,
  content: string = 'The sky is blue on clear days.',
  opts: {
    source_quality?: number;
    classification?: Record<string, unknown>;
    labels?: string[];
  } = {},
): Promise<{ id: string; fragment_id: string; [k: string]: unknown }> {
  const res = await request.post(`${BASE_URL}/api/v1/fragments`, {
    headers: headers(profileId),
    data: {
      content,
      source_quality: opts.source_quality ?? 0.9,
      classification: opts.classification ?? { domain: 'science', confidence: 0.9 },
      labels: opts.labels ?? ['fact', 'science'],
    },
  });
  expect([200, 201], `seedFragment: expected 200 or 201, got ${res.status()}`).toContain(res.status());
  const body = await res.json();
  return body as { id: string; fragment_id: string };
}

/**
 * Create a claim and then verify it (PUT to validated state).
 * Returns the parsed claim body after verification.
 */
export async function createAndVerifyClaim(
  request: APIRequestContext,
  profileId: string,
  opts: {
    predicate?: string;
    subject?: string;
    object?: string;
    fragmentId?: string;
    verifier_model?: string;
    modality?: string;
    extract_conf?: number;
    resolution_conf?: number;
  } = {},
): Promise<{ id: string; status: string; [k: string]: unknown }> {
  // If no fragment provided, seed one first
  let fragmentId = opts.fragmentId;
  if (!fragmentId) {
    const frag = await seedFragmentForProfile(request, profileId);
    fragmentId = frag.fragment_id;
  }

  // Create claim
  const createRes = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: opts.predicate ?? 'likes',
      subject: opts.subject ?? 'sky',
      object: opts.object ?? 'blue',
      modality: opts.modality ?? 'assertion',
      extract_conf: opts.extract_conf ?? 0.95,
      resolution_conf: opts.resolution_conf ?? 0.95,
      supported_by: [fragmentId],
    },
  });
  expect([200, 201], `createClaim: expected 200 or 201, got ${createRes.status()}`).toContain(createRes.status());
  const createBody = await createRes.json();
  const claimId: string = createBody.claim_id;

  // Verify claim
  const verifyRes = await request.post(
    `${BASE_URL}/api/v1/claims/${claimId}/verify`,
    {
      headers: headers(profileId),
      data: {
        verifier_model: opts.verifier_model ?? 'test-verifier',
      },
    },
  );
  expect(verifyRes.status(), `verifyClaim: expected 200, got ${verifyRes.status()}`).toBe(200);
  const verifyBody = await verifyRes.json();
  return {
    id: verifyBody.claim_id as string,
    claim_id: verifyBody.claim_id as string,
    status: verifyBody.status as string,
    entailment_verdict: verifyBody.entailment_verdict as string,
  };
}

/**
 * Create, verify, and promote a claim to a fact.
 * Returns the parsed fact body.
 */
export async function createAndPromoteClaim(
  request: APIRequestContext,
  profileId: string,
  opts: {
    predicate?: string;
    subject?: string;
    object?: string;
    fragmentId?: string;
    policy?: string;
  } = {},
): Promise<{ id: string; [k: string]: unknown }> {
  const claim = await createAndVerifyClaim(request, profileId, {
    predicate: opts.predicate,
    subject: opts.subject,
    object: opts.object,
    fragmentId: opts.fragmentId,
  });

  const promoteRes = await request.post(
    `${BASE_URL}/api/v1/claims/${claim.id}/promote`,
    {
      headers: headers(profileId),
      data: {
        policy: opts.policy ?? 'single_supporter',
      },
    },
  );
  expect(promoteRes.status(), `promoteClaim: expected 201, got ${promoteRes.status()}`).toBe(201);
  const promoteBody = await promoteRes.json();
  return {
    id: promoteBody.fact_id as string,
    fact_id: promoteBody.fact_id as string,
    ...promoteBody,
  } as { id: string };
}

/**
 * Create two validated+promoted facts that could trigger contradiction detection.
 * Returns [factA, factB].
 */
export async function createTwoSupportPromotedFact(
  request: APIRequestContext,
  profileId: string,
  sharedSubject: string = 'water_boiling_point',
): Promise<[{ id: string }, { id: string }]> {
  const factA = await createAndPromoteClaim(request, profileId, {
    subject: sharedSubject,
    predicate: 'IS',
    object: '100_celsius',
  });
  const factB = await createAndPromoteClaim(request, profileId, {
    subject: sharedSubject,
    predicate: 'IS',
    object: '212_fahrenheit',
  });
  return [factA, factB];
}

/**
 * Create a validated (but not yet promoted) claim for verify-stage tests.
 * Returns the claim body with status=validated.
 */
export async function createValidatedCandidateForVerify(
  request: APIRequestContext,
  profileId: string,
  opts: {
    predicate?: string;
    subject?: string;
    object?: string;
  } = {},
): Promise<{ id: string; status: string; [k: string]: unknown }> {
  return createAndVerifyClaim(request, profileId, opts);
}
