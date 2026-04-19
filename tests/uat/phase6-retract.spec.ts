/**
 * UAT-09 — Phase 6: Fragment retraction.
 *
 * Verifies that POST /api/v1/fragments/:id/retract:
 * - marks a fragment as retracted
 * - cascades retraction to any claims that relied solely on the retracted fragment
 * - is profile-isolated (profile B cannot retract profile A's fragments)
 * - returns stable error codes for unknown/already-retracted fragments
 *
 * Will be RED until Units 45-50 (retract service, retract handler, cascade) are complete.
 */

import { test, expect } from '@playwright/test';
import {
  headers,
  seedFragmentForProfile,
  BASE_URL,
} from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase6-retract';
const profileIdB = 'uat-profile-phase6-retract-b';

// UAT-09a: Retract a fragment returns 200 and marks it retracted
test('UAT-09a: POST /fragments/:id/retract returns 200 and retracted state', async ({
  request,
}) => {
  const frag = await seedFragmentForProfile(request, profileId, 'Fragment to be retracted.');

  const res = await request.post(`${BASE_URL}/api/v1/fragments/${frag.id}/retract`, {
    headers: headers(profileId),
  });
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body).toHaveProperty('data');
  expect(body.data).toMatchObject({
    id: frag.id,
    retracted: true,
  });
});

// UAT-09b: Retracting an already-retracted fragment is idempotent (200)
test('UAT-09b: retracting an already-retracted fragment is idempotent', async ({
  request,
}) => {
  const frag = await seedFragmentForProfile(request, profileId, 'Fragment retracted twice.');

  // First retraction
  const res1 = await request.post(`${BASE_URL}/api/v1/fragments/${frag.id}/retract`, {
    headers: headers(profileId),
  });
  expect(res1.status()).toBe(200);

  // Second retraction — must be idempotent
  const res2 = await request.post(`${BASE_URL}/api/v1/fragments/${frag.id}/retract`, {
    headers: headers(profileId),
  });
  expect([200, 409]).toContain(res2.status());
});

// UAT-09c: Retracting a non-existent fragment returns 404
test('UAT-09c: retracting non-existent fragment returns 404', async ({ request }) => {
  const res = await request.post(
    `${BASE_URL}/api/v1/fragments/00000000-0000-0000-0000-000000000000/retract`,
    {
      headers: headers(profileId),
    },
  );
  expect(res.status()).toBe(404);
  const body = await res.json();
  expect(body).toHaveProperty('code');
  expect(typeof body.code).toBe('string');
});

// UAT-09d: Cross-profile isolation — profile B cannot retract profile A's fragment
test('UAT-09d: cross-profile isolation — profile B cannot retract profile A fragment', async ({
  request,
}) => {
  const frag = await seedFragmentForProfile(
    request,
    profileId,
    'Profile A fragment — must not be retractable by profile B.',
  );

  const res = await request.post(`${BASE_URL}/api/v1/fragments/${frag.id}/retract`, {
    headers: headers(profileIdB),
  });
  // Must return 404 or 403, not 200
  expect(res.status()).not.toBe(200);
  expect(res.status()).toBeGreaterThanOrEqual(400);
});

// UAT-09e: Claims backed solely by a retracted fragment are cascaded to retracted
test('UAT-09e: claim backed by retracted fragment cascades to retracted', async ({
  request,
}) => {
  const frag = await seedFragmentForProfile(
    request,
    profileId,
    'Sole supporting fragment for cascade test.',
  );

  // Create a claim backed only by this fragment
  const createRes = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'cascade_subject',
      object: 'cascade_object',
      supporting_fragment_ids: [frag.fragment_id],
    },
  });
  expect(createRes.status()).toBe(201);
  const claimId: string = (await createRes.json()).data.id;

  // Retract the supporting fragment
  const retractRes = await request.post(`${BASE_URL}/api/v1/fragments/${frag.id}/retract`, {
    headers: headers(profileId),
  });
  expect(retractRes.status()).toBe(200);

  // The claim should now be retracted or invalidated
  const claimRes = await request.get(`${BASE_URL}/api/v1/claims/${claimId}`, {
    headers: headers(profileId),
  });
  if (claimRes.status() === 200) {
    const body = await claimRes.json();
    // Claim must be in a non-active state after its sole fragment was retracted
    expect(['retracted', 'invalidated', 'rejected']).toContain(body.data.status);
  } else {
    // 404 is also acceptable — claim may be physically removed
    expect(claimRes.status()).toBe(404);
  }
});
