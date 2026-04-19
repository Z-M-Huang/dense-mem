/**
 * UAT-01 — Phase 1: Neo4j vector and constraint indexes exist.
 *
 * Verifies that the server bootstraps the required Neo4j schema:
 * - SourceFragment vector index (`source_fragment_embedding`)
 * - Fact vector index (`fact_embedding`)
 * - Uniqueness constraints on SourceFragment and Fact nodes
 *
 * These tests exercise the admin graph/query surface and the schema
 * bootstrapper. They will be RED until Unit 5 (schema bootstrapper) and
 * Unit 58 (server wiring) are complete.
 */

import { test, expect } from '@playwright/test';
import { adminHeaders, neo4jQuery, BASE_URL } from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase1';

// UAT-01a: Vector index on SourceFragment exists in Neo4j
test('UAT-01a: SourceFragment vector index exists', async ({ request }) => {
  const res = await request.post(`${BASE_URL}/api/v1/admin/graph/query`, {
    headers: adminHeaders(),
    data: {
      cypher: "SHOW VECTOR INDEXES YIELD name WHERE name = 'source_fragment_embedding' RETURN name",
    },
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body).toHaveProperty('data');
  const rows: unknown[] = body.data.rows ?? body.data;
  expect(rows.length).toBeGreaterThan(0);
});

// UAT-01b: Vector index on Fact exists in Neo4j
test('UAT-01b: Fact vector index exists', async ({ request }) => {
  const res = await request.post(`${BASE_URL}/api/v1/admin/graph/query`, {
    headers: adminHeaders(),
    data: {
      cypher: "SHOW VECTOR INDEXES YIELD name WHERE name = 'fact_embedding' RETURN name",
    },
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  const rows: unknown[] = body.data.rows ?? body.data;
  expect(rows.length).toBeGreaterThan(0);
});

// UAT-01c: Uniqueness constraint on SourceFragment.id exists
test('UAT-01c: SourceFragment uniqueness constraint exists', async ({ request }) => {
  const res = await request.post(`${BASE_URL}/api/v1/admin/graph/query`, {
    headers: adminHeaders(),
    data: {
      cypher:
        "SHOW CONSTRAINTS YIELD name, labelsOrTypes " +
        "WHERE 'SourceFragment' IN labelsOrTypes RETURN name",
    },
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  const rows: unknown[] = body.data.rows ?? body.data;
  expect(rows.length).toBeGreaterThan(0);
});

// UAT-01d: Uniqueness constraint on Claim.id exists
test('UAT-01d: Claim uniqueness constraint exists', async ({ request }) => {
  const res = await request.post(`${BASE_URL}/api/v1/admin/graph/query`, {
    headers: adminHeaders(),
    data: {
      cypher:
        "SHOW CONSTRAINTS YIELD name, labelsOrTypes " +
        "WHERE 'Claim' IN labelsOrTypes RETURN name",
    },
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  const rows: unknown[] = body.data.rows ?? body.data;
  expect(rows.length).toBeGreaterThan(0);
});

// UAT-01e: Uniqueness constraint on Fact.id exists
test('UAT-01e: Fact uniqueness constraint exists', async ({ request }) => {
  const res = await request.post(`${BASE_URL}/api/v1/admin/graph/query`, {
    headers: adminHeaders(),
    data: {
      cypher:
        "SHOW CONSTRAINTS YIELD name, labelsOrTypes " +
        "WHERE 'Fact' IN labelsOrTypes RETURN name",
    },
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  const rows: unknown[] = body.data.rows ?? body.data;
  expect(rows.length).toBeGreaterThan(0);
});

// AC-X2 regression: OpenAPI schema enumerates claim and fact routes
test('AC-X2 regression: OpenAPI lists /claims and /facts routes', async ({ request }) => {
  const res = await request.get(`${BASE_URL}/api/v1/openapi.json`, {
    headers: {
      'Authorization': `Bearer ${process.env.API_KEY || 'test-api-key'}`,
      'X-Profile-ID': profileId,
    },
  });
  expect(res.status()).toBe(200);
  const spec = await res.json();
  expect(spec).toHaveProperty('paths');
  const paths: Record<string, unknown> = spec.paths;
  expect(Object.keys(paths)).toEqual(
    expect.arrayContaining([
      expect.stringMatching('/claims'),
      expect.stringMatching('/facts'),
    ]),
  );
});
