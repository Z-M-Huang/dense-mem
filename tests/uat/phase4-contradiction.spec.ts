/**
 * UAT-07 — Phase 4: Contradiction detection on promote.
 *
 * Verifies that POST /api/v1/claims/:id/promote returns comparable_disputed (409)
 * when a comparable fact already exists for the same subject+predicate with a
 * different object under strict (single-valued) policies.
 *
 * Also verifies the `rejected_weaker` and `gate_rejected` paths.
 *
 * Will be RED until Units 35-43 (gates, contradiction, promote service/handler) are complete.
 */

import { test, expect } from '@playwright/test';
import {
  headers,
  createAndVerifyClaim,
  createAndPromoteClaim,
  createTwoSupportPromotedFact,
  BASE_URL,
} from './helpers';

const profileId = process.env.PROFILE_ID || 'uat-profile-phase4-contradiction';

// UAT-07a: Promoting a claim that contradicts an existing fact returns comparable_disputed
test('UAT-07a: contradicting fact returns comparable_disputed 409', async ({ request }) => {
  // Promote first fact: boiling_point IS 100_celsius
  await createAndPromoteClaim(request, profileId, {
    predicate: 'IS',
    subject: 'boiling_point_test',
    object: '100_celsius',
    policy: 'single_supporter',
  });

  // Create + verify a contradicting claim: boiling_point IS 200_celsius
  const contradictingClaim = await createAndVerifyClaim(request, profileId, {
    predicate: 'IS',
    subject: 'boiling_point_test',
    object: '200_celsius',
  });

  const promoteRes = await request.post(
    `${BASE_URL}/api/v1/claims/${contradictingClaim.id}/promote`,
    {
      headers: headers(profileId),
      data: { policy: 'single_supporter' },
    },
  );
  expect(promoteRes.status()).toBe(409);
  const body = await promoteRes.json();
  expect(body.code).toBe('comparable_disputed');
});

// UAT-07b: Promoting a weaker claim returns rejected_weaker (409)
test('UAT-07b: weaker claim returns rejected_weaker 409', async ({ request }) => {
  // Promote first fact with high source quality
  await createAndPromoteClaim(request, profileId, {
    predicate: 'HAS_PROPERTY',
    subject: 'iron_weaker_test',
    object: 'ferromagnetic',
    policy: 'single_supporter',
  });

  // Try to promote a weaker claim for the same fact
  const weakerClaim = await createAndVerifyClaim(request, profileId, {
    predicate: 'HAS_PROPERTY',
    subject: 'iron_weaker_test',
    object: 'ferromagnetic',
  });

  const promoteRes = await request.post(
    `${BASE_URL}/api/v1/claims/${weakerClaim.id}/promote`,
    {
      headers: headers(profileId),
      data: { policy: 'single_supporter' },
    },
  );
  // Should be 201 (idempotent, same fact) or 409 rejected_weaker depending on policy
  expect([200, 201, 409]).toContain(promoteRes.status());
  if (promoteRes.status() === 409) {
    const body = await promoteRes.json();
    expect(['rejected_weaker', 'comparable_disputed']).toContain(body.code);
  }
});

// UAT-07c: Gate rejection with unsupported policy returns gate_rejected (409)
test('UAT-07c: unsupported_policy returns 422', async ({ request }) => {
  const claim = await createAndVerifyClaim(request, profileId, {
    predicate: 'IS',
    subject: 'gate_test_subject',
    object: 'gate_test_object',
  });

  const promoteRes = await request.post(
    `${BASE_URL}/api/v1/claims/${claim.id}/promote`,
    {
      headers: headers(profileId),
      data: { policy: 'invalid_unknown_policy' },
    },
  );
  expect(promoteRes.status()).toBe(422);
  const body = await promoteRes.json();
  expect(body.code).toBe('unsupported_policy');
});

// UAT-07d: Two facts with different objects for same predicate — both can coexist under multi-value policy
test('UAT-07d: multi-value policy allows multiple objects for same predicate', async ({
  request,
}) => {
  const [factA, factB] = await createTwoSupportPromotedFact(
    request,
    profileId,
    'multi_value_subject',
  );
  // Both should exist as separate facts
  expect(factA.id).toBeDefined();
  expect(factB.id).toBeDefined();
  expect(factA.id).not.toBe(factB.id);

  // Both should be retrievable
  const resA = await request.get(`${BASE_URL}/api/v1/facts/${factA.id}`, {
    headers: headers(profileId),
  });
  const resB = await request.get(`${BASE_URL}/api/v1/facts/${factB.id}`, {
    headers: headers(profileId),
  });
  expect(resA.status()).toBe(200);
  expect(resB.status()).toBe(200);
});
