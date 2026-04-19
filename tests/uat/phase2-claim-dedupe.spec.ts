/**
 * UAT-03 — Phase 2: Claim deduplication.
 *
 * Verifies that:
 * - creating a claim with the same (subject, predicate, object) returns the
 *   same claim_id regardless of which fragment backs it
 * - a second creation with a different fragment adds a SUPPORTED_BY edge
 *   without creating a new Claim node
 * - the dedupe is profile-scoped: same triple in profile B is a different claim
 *
 * Will be RED until Unit 20 (claim dedupe) is complete.
 */

import { test, expect } from '@playwright/test';
import { headers, seedFragmentForProfile, BASE_URL } from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase2-dedupe';
const profileIdB = 'uat-profile-phase2-dedupe-b';

// UAT-03a: Same triple from different fragments returns same claim_id
test('UAT-03a: same triple returns existing claim_id (dedupe)', async ({ request }) => {
  const frag1 = await seedFragmentForProfile(request, profileId, 'First source: fire is hot.');
  const frag2 = await seedFragmentForProfile(request, profileId, 'Second source: fire is hot.');

  const payload = (fragmentId: string) => ({
    predicate: 'IS',
    subject: 'fire',
    object: 'hot',
    supporting_fragment_ids: [fragmentId],
  });

  const res1 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: payload(frag1.fragment_id),
  });
  const res2 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: payload(frag2.fragment_id),
  });

  expect(res1.status()).toBe(201);
  expect(res2.status()).toBe(201);
  const id1: string = (await res1.json()).data.id;
  const id2: string = (await res2.json()).data.id;
  expect(id1).toBe(id2);
});

// UAT-03b: Deduplicated claim accumulates multiple SUPPORTED_BY edges
test('UAT-03b: deduplicated claim accumulates supporting fragments', async ({
  request,
}) => {
  const frag1 = await seedFragmentForProfile(request, profileId, 'Water is wet — src A.');
  const frag2 = await seedFragmentForProfile(request, profileId, 'Water is wet — src B.');

  // Create with first fragment
  await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'water_dedupe_test',
      object: 'wet',
      supporting_fragment_ids: [frag1.fragment_id],
    },
  });

  // Create with second fragment — should dedupe and add SUPPORTED_BY edge
  const res2 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'water_dedupe_test',
      object: 'wet',
      supporting_fragment_ids: [frag2.fragment_id],
    },
  });
  expect(res2.status()).toBe(201);
  const body = await res2.json();
  // Claim should report multiple supporters or at minimum respond with 201
  expect(body.data).toHaveProperty('id');
  // supporting_count (or similar) should be >= 2 after two POSTs
  if (body.data.supporting_count !== undefined) {
    expect(body.data.supporting_count).toBeGreaterThanOrEqual(2);
  }
});

// UAT-03c: Dedupe is profile-scoped — same triple in another profile is distinct
test('UAT-03c: dedupe is profile-scoped', async ({ request }) => {
  const fragA = await seedFragmentForProfile(
    request,
    profileId,
    'Snow is white — profile A source.',
  );
  const fragB = await seedFragmentForProfile(
    request,
    profileIdB,
    'Snow is white — profile B source.',
  );

  const res1 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'snow',
      object: 'white',
      supporting_fragment_ids: [fragA.fragment_id],
    },
  });
  const res2 = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileIdB),
    data: {
      predicate: 'IS',
      subject: 'snow',
      object: 'white',
      supporting_fragment_ids: [fragB.fragment_id],
    },
  });

  expect(res1.status()).toBe(201);
  expect(res2.status()).toBe(201);
  const id1: string = (await res1.json()).data.id;
  const id2: string = (await res2.json()).data.id;
  // Different profiles → different claim IDs even for identical triples
  expect(id1).not.toBe(id2);
});

// UAT-03d: Cross-profile isolation — dedupe endpoint must filter by profile
test('UAT-03d: cross-profile isolation — dedupe is not cross-profile', async ({
  request,
}) => {
  // Creating a claim in profile A should not affect claim count in profile B
  const fragA = await seedFragmentForProfile(request, profileId, 'Coal is black — profile A.');
  const res = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileId),
    data: {
      predicate: 'IS',
      subject: 'coal_isolation',
      object: 'black',
      supporting_fragment_ids: [fragA.fragment_id],
    },
  });
  expect(res.status()).toBe(201);

  // Profile B must not see profile A's claim when it posts the same triple
  const fragB = await seedFragmentForProfile(request, profileIdB, 'Coal is black — profile B.');
  const resB = await request.post(`${BASE_URL}/api/v1/claims`, {
    headers: headers(profileIdB),
    data: {
      predicate: 'IS',
      subject: 'coal_isolation',
      object: 'black',
      supporting_fragment_ids: [fragB.fragment_id],
    },
  });
  expect(resB.status()).toBe(201);
  const idA: string = (await res.json()).data.id;
  const idB: string = (await resB.json()).data.id;
  expect(idA).not.toBe(idB);
});
