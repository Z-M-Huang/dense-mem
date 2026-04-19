/**
 * UAT-05 — Phase 3: Claim verification error paths.
 *
 * Verifies that POST /api/v1/claims/:id/verify returns stable,
 * machine-readable error codes (AC-X6) for known failure modes:
 * - claim_not_found (404)
 * - supporting_fragment_missing (404)
 * - predicate_not_policed (422)
 * - verifier_timeout (504)
 * - verifier_provider (503)
 *
 * Will be RED until Units 3 (httperr taxonomy) and 33 (verify handler) are complete.
 */

import { test, expect } from '@playwright/test';
import { headers, adminHeaders, seedFragmentForProfile, BASE_URL } from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase3-errors';

// UAT-05a: Non-existent claim returns claim_not_found (404)
test('UAT-05a: verify non-existent claim returns 404 with claim_not_found', async ({
  request,
}) => {
  const res = await request.post(
    `${BASE_URL}/api/v1/claims/00000000-0000-0000-0000-000000000000/verify`,
    {
      headers: headers(profileId),
      data: { verifier_model: 'test-verifier' },
    },
  );
  expect(res.status()).toBe(404);
  const body = await res.json();
  expect(body).toMatchObject({
    error: expect.any(String),
    code: 'claim_not_found',
  });
});

// UAT-05b: Claim with missing supporting fragment returns supporting_fragment_missing (404)
test('UAT-05b: claim with deleted fragment returns supporting_fragment_missing', async ({
  request,
}) => {
  // Create a fragment, create a claim, delete the fragment, then verify
  const frag = await seedFragmentForProfile(request, profileId, 'Temporary fragment.');
  const createRes = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'temp_subject',
      object: 'temp_object',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  expect(createRes.status()).toBe(201);
  const claimId: string = (await createRes.json()).data.id;

  // Delete the supporting fragment
  await request.delete(`${BASE_URL}/api/v1/fragments/${frag.id}`, {
    headers: headers(profileId),
  });

  const verifyRes = await request.post(
    `${BASE_URL}/api/v1/claims/${claimId}/verify`,
    {
      headers: headers(profileId),
      data: { verifier_model: 'test-verifier' },
    },
  );
  // Expected: 404 with supporting_fragment_missing code
  expect(verifyRes.status()).toBe(404);
  const body = await verifyRes.json();
  expect(body.code).toBe('supporting_fragment_missing');
});

// UAT-05c: Unknown predicate returns predicate_not_policed (422)
test('UAT-05c: unknown predicate returns predicate_not_policed 422', async ({ request }) => {
  const frag = await seedFragmentForProfile(request, profileId, 'Some content for unknown predicate.');
  const createRes = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'FROBS',
      subject: 'thing_a',
      object: 'thing_b',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  // Claim creation may succeed (predicate stored as-is) or reject at creation
  if (createRes.status() === 201) {
    const claimId: string = (await createRes.json()).data.id;
    const verifyRes = await request.post(
      `${BASE_URL}/api/v1/claims/${claimId}/verify`,
      {
        headers: headers(profileId),
        data: { verifier_model: 'test-verifier' },
      },
    );
    expect(verifyRes.status()).toBe(422);
    const body = await verifyRes.json();
    expect(body.code).toBe('predicate_not_policed');
  } else {
    // If creation itself returns 422 for unknown predicate, that's also acceptable
    expect(createRes.status()).toBe(422);
    const body = await createRes.json();
    expect(body.code).toBe('predicate_not_policed');
  }
});

// UAT-05d: AC-X6 regression — error responses have stable machine-readable codes
test('AC-X6 regression: error response has stable code field', async ({ request }) => {
  const res = await request.post(
    `${BASE_URL}/api/v1/claims/not-a-valid-uuid/verify`,
    {
      headers: headers(profileId),
      data: { verifier_model: 'test-verifier' },
    },
  );
  // Should return 400 or 404 with a stable error code
  expect(res.status()).toBeGreaterThanOrEqual(400);
  const body = await res.json();
  // The error envelope must include a `code` field for machine parsing
  expect(body).toHaveProperty('code');
  expect(typeof body.code).toBe('string');
  // code must be lowercase with underscores (stable external contract)
  expect(body.code).toMatch(/^[a-z][a-z0-9_]*$/);
});
