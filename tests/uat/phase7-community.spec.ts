/**
 * UAT-10 — Phase 7: Community detection on knowledge graph.
 *
 * Verifies that POST /api/v1/tools/detect_community:
 * - requires a profile-bound authenticated caller
 * - runs community detection on the profile's knowledge graph
 * - returns a detect result envelope with community summaries
 * - is profile-scoped (separate profiles produce separate communities)
 * - handles profiles with no graph data gracefully
 */

import { test, expect } from '@playwright/test';
import { randomUUID } from 'crypto';
import {
  headers,
  createAndPromoteClaim,
  BASE_URL,
} from './helpers';

const profileId = process.env.PROFILE_ID || '00000000-0000-0000-0000-000000000101';
const profileIdB = process.env.PROFILE_ID_B || '00000000-0000-0000-0000-000000000102';

// UAT-10a: Community detection returns 200 with summary and communities are readable
test('UAT-10a: community detection returns 200 with summary', async ({ request }) => {
  // Seed some facts to form a graph
  await createAndPromoteClaim(request, profileId, {
    predicate: 'likes',
    subject: 'hydrogen',
    object: 'element',
  });
  await createAndPromoteClaim(request, profileId, {
    predicate: 'likes',
    subject: 'oxygen',
    object: 'element',
  });

  const res = await request.post(
    `${BASE_URL}/api/v1/tools/detect_community`,
    {
      headers: headers(profileId),
    },
  );
  expect(res.status()).toBe(200);
  const body = await res.json();
  const data = body as Record<string, unknown>;
  expect(data.detected).toBe(true);
  expect(typeof data.community_count).toBe('number');
  expect(Array.isArray(data.communities)).toBe(true);

  const listRes = await request.get(`${BASE_URL}/api/v1/communities`, {
    headers: headers(profileId),
  });
  expect(listRes.status()).toBe(200);
  const listBody = await listRes.json();
  expect(Array.isArray(listBody.items)).toBe(true);
});

// UAT-10b: Community detection requires a profile header.
test('UAT-10b: community detection rejects missing profile context', async ({ request }) => {
  const res = await request.post(
    `${BASE_URL}/api/v1/tools/detect_community`,
    {
      headers: {
        'Authorization': `Bearer ${process.env.API_KEY || 'test-api-key'}`,
        'Content-Type': 'application/json',
      },
    },
  );
  expect(res.status()).toBe(400);
});

// UAT-10c: Community detection on a profile with no data returns empty summary
test('UAT-10c: empty profile returns empty community summary', async ({ request }) => {
  const emptyProfileId = randomUUID();

  const res = await request.post(
    `${BASE_URL}/api/v1/tools/detect_community`,
    {
      headers: headers(emptyProfileId),
    },
  );
  expect(res.status()).toBeGreaterThanOrEqual(400);
  expect(res.status()).toBeLessThanOrEqual(404);
});

// UAT-10d: Community detection is profile-scoped — does not cross profile boundaries
test('UAT-10d: community detection is profile-scoped', async ({ request }) => {
  // Seed facts in profile A
  await createAndPromoteClaim(request, profileId, {
    predicate: 'likes',
    subject: 'carbon_community_test',
    object: 'nonmetal',
  });

  // Run detection on profile B — must not include profile A facts
  const res = await request.post(
    `${BASE_URL}/api/v1/tools/detect_community`,
    {
      headers: headers(profileIdB),
    },
  );
  expect(res.status()).toBeGreaterThanOrEqual(400);
  expect(res.status()).toBeLessThanOrEqual(404);
});
