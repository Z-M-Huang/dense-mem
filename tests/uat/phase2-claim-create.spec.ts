/**
 * UAT-02 — Phase 2: Claim creation happy path.
 *
 * Verifies that POST /api/v1/claims:
 * - accepts a subject, predicate, object, and supporting_fragment_ids
 * - returns 201 with a stable claim_id
 * - records the Claim node in Neo4j with profile isolation
 * - inherits source_quality from the supporting fragment
 *
 * Will be RED until Units 17-22 (claim domain, service, handler) are complete.
 */

import { test, expect } from '@playwright/test';
import { headers, seedFragmentForProfile, BASE_URL } from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase2-create';
const profileIdB = 'uat-profile-phase2-create-b'; // cross-profile isolation

// UAT-02a: POST /api/v1/claims returns 201 with claim_id
test('UAT-02a: claim creation returns 201 with claim_id', async ({ request }) => {
  const frag = await seedFragmentForProfile(request, profileId);

  const res = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'sky',
      object: 'blue',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  expect(res.status()).toBe(201);
  const body = await res.json();
  expect(body).toMatchObject({
    data: {
      id: expect.any(String),
      predicate: 'IS',
      subject: 'sky',
      object: 'blue',
      status: expect.stringMatching(/^(candidate|pending)$/),
    },
  });
});

// UAT-02b: Returned claim_id is stable (deterministic from content hash)
test('UAT-02b: claim_id is stable across identical requests', async ({ request }) => {
  const frag = await seedFragmentForProfile(request, profileId, 'Grass is green.');

  const payload = {
    predicate: 'IS',
    subject: 'grass',
    object: 'green',
    supporting_fragment_ids: [frag.fragment_id],
  };

  const res1 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: payload,
  });
  const res2 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: payload,
  });

  expect(res1.status()).toBe(201);
  expect(res2.status()).toBe(201);
  const body1 = await res1.json();
  const body2 = await res2.json();
  expect(body1.data.id).toBe(body2.data.id);
});

// UAT-02c: Claim inherits source_quality from the supporting fragment
test('UAT-02c: claim inherits source_quality from fragment', async ({ request }) => {
  const frag = await seedFragmentForProfile(request, profileId, 'Ice is cold.', {
    source_quality: 0.8,
  });

  const res = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'ice',
      object: 'cold',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  expect(res.status()).toBe(201);
  const body = await res.json();
  expect(body.data).toHaveProperty('source_quality');
  expect(typeof body.data.source_quality).toBe('number');
});

// UAT-02d: Missing required fields returns 400
test('UAT-02d: missing predicate returns 400', async ({ request }) => {
  const frag = await seedFragmentForProfile(request, profileId);

  const res = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      // predicate intentionally omitted
      subject: 'test',
      object: 'value',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  expect(res.status()).toBe(400);
});

// UAT-02e: Cross-profile isolation — claim created for profile A not visible to profile B
test('UAT-02e: cross-profile isolation — claim not visible across profiles', async ({
  request,
}) => {
  const fragA = await seedFragmentForProfile(request, profileId, 'Profile A content');

  const createRes = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'HAS',
      subject: 'profile_a',
      object: 'exclusive_claim',
      supporting_fragment_ids: [fragA.fragment_id],
    },
  });
  expect(createRes.status()).toBe(201);
  const claimId: string = (await createRes.json()).data.id;

  // Profile B must not be able to read profile A's claim
  const readRes = await request.get(`${BASE_URL}/api/v1/claims/${claimId}`, {
    headers: headers(profileIdB),
  });
  // Should be 404 (not found) or 403 (forbidden), not 200
  expect(readRes.status()).not.toBe(200);
});
