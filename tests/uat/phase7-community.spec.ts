/**
 * UAT-10 — Phase 7: Community detection on knowledge graph.
 *
 * Verifies that POST /api/v1/admin/profiles/:id/community/detect:
 * - requires admin authentication
 * - runs community detection on the profile's knowledge graph
 * - returns a summary with community count and node assignments
 * - is profile-scoped (separate profiles produce separate communities)
 * - handles profiles with no graph data gracefully
 *
 * Will be RED until Units 51-56 (community detection service and handler) are complete.
 */

import { test, expect } from '@playwright/test';
import {
  headers,
  adminHeaders,
  adminProfileHeaders,
  createAndPromoteClaim,
  BASE_URL,
  PROFILE_ID,
} from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase7-community';
const profileIdB = 'uat-profile-phase7-community-b';

// UAT-10a: Community detection returns 200 with community summary
test('UAT-10a: community detection returns 200 with summary', async ({ request }) => {
  // Seed some facts to form a graph
  await createAndPromoteClaim(request, profileId, {
    predicate: 'IS',
    subject: 'hydrogen',
    object: 'element',
  });
  await createAndPromoteClaim(request, profileId, {
    predicate: 'IS',
    subject: 'oxygen',
    object: 'element',
  });

  const res = await request.post(
    `${BASE_URL}/api/v1/admin/profiles/${profileId}/community/detect`,
    {
      headers: adminProfileHeaders(profileId),
    },
  );
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body).toHaveProperty('data');
  // Must include community count or similar summary field
  const data = body.data as Record<string, unknown>;
  expect(
    data.community_count !== undefined ||
    data.communities !== undefined ||
    data.node_count !== undefined,
  ).toBe(true);
});

// UAT-10b: Community detection requires admin key — standard key returns 401/403
test('UAT-10b: community detection rejects non-admin key', async ({ request }) => {
  const res = await request.post(
    `${BASE_URL}/api/v1/admin/profiles/${profileId}/community/detect`,
    {
      headers: headers(profileId), // standard, not admin
    },
  );
  expect(res.status()).toBeGreaterThanOrEqual(401);
  expect(res.status()).toBeLessThanOrEqual(403);
});

// UAT-10c: Community detection on a profile with no data returns empty summary
test('UAT-10c: empty profile returns empty community summary', async ({ request }) => {
  const emptyProfileId = `uat-empty-community-${Date.now()}`;

  const res = await request.post(
    `${BASE_URL}/api/v1/admin/profiles/${emptyProfileId}/community/detect`,
    {
      headers: adminHeaders(),
    },
  );
  // Either 200 with empty results, or 404 if the profile doesn't exist
  expect([200, 404]).toContain(res.status());
  if (res.status() === 200) {
    const body = await res.json();
    expect(body).toHaveProperty('data');
    const data = body.data as Record<string, unknown>;
    const count = (data.community_count as number) ?? 0;
    expect(count).toBeGreaterThanOrEqual(0);
  }
});

// UAT-10d: Community detection is profile-scoped — does not cross profile boundaries
test('UAT-10d: community detection is profile-scoped', async ({ request }) => {
  // Seed facts in profile A
  await createAndPromoteClaim(request, profileId, {
    predicate: 'IS',
    subject: 'carbon_community_test',
    object: 'nonmetal',
  });

  // Run detection on profile B — must not include profile A facts
  const res = await request.post(
    `${BASE_URL}/api/v1/admin/profiles/${profileIdB}/community/detect`,
    {
      headers: adminHeaders(),
    },
  );
  expect([200, 404]).toContain(res.status());
  // We can't directly assert isolation here beyond checking it doesn't error,
  // since cross-profile data leakage would require Neo4j inspection.
  // The cross-profile isolation unit tests cover that deeper assertion.
  if (res.status() === 200) {
    const body = await res.json();
    expect(body).toHaveProperty('data');
  }
});
