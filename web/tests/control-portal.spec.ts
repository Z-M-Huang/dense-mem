import { expect, Page, test } from "@playwright/test";

const profile = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Default",
  description: "",
  metadata: null,
  config: null,
  created_at: "2026-05-01T12:00:00Z",
  updated_at: "2026-05-01T12:00:00Z",
};

const key = {
  id: "22222222-2222-4222-8222-222222222222",
  profile_id: profile.id,
  key_suffix: "abc123",
  rate_limit: 120,
  last_used_at: "2026-05-02T13:00:00Z",
  expires_at: null,
  created_at: "2026-04-30T12:00:00Z",
};

type TestProfile = typeof profile;
type TestKey = typeof key;

test("profile creation flow", async ({ page }) => {
  await mockApi(page, { profiles: [profile], keys: [] });
  await openPortal(page);

  await page.getByLabel("Name").first().fill("Work Profile");
  await page.getByLabel("Description").first().fill("for work");
  await page.getByRole("button", { name: /^Create$/ }).click();

  await expect(page.getByRole("heading", { name: "Work Profile" })).toBeVisible();
});

test("API key creation shows plaintext once", async ({ page }) => {
  await mockApi(page, { profiles: [profile], keys: [] });
  await openPortal(page);

  await page.getByRole("button", { name: "Create key" }).click();

  await expect(page.getByText("dm_plain_once")).toBeVisible();
  await page.getByRole("button", { name: "Dismiss API key" }).click();
  await expect(page.getByText("dm_plain_once")).toBeHidden();
});

test("API key list and delete flow", async ({ page }) => {
  await mockApi(page, { profiles: [profile], keys: [key] });
  await openPortal(page);

  await expect(page.getByText("******abc123")).toBeVisible();
  const keyRow = page.getByRole("row", { name: /abc123/ });
  await expect(keyRow.getByText(/May/)).toBeVisible();
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: /Delete API key \*\*\*\*\*\*abc123/ }).click();

  await expect(page.getByRole("button", { name: /Delete API key \*\*\*\*\*\*abc123/ })).toBeHidden();
  await expect(page.getByText("******abc123")).toBeHidden();
});

test("profile delete flow", async ({ page }) => {
  await mockApi(page, { profiles: [profile], keys: [key] });
  await openPortal(page);

  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: /^Delete$/ }).click();

  await expect(page.getByText("No profiles")).toBeVisible();
});

test("auth token failure", async ({ page }) => {
  await page.route("**/control/api/session", async (route) => {
    await route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ message: "invalid token" }) });
  });
  await page.goto("/");

  await page.getByLabel("Control token").fill("wrong");
  await page.getByRole("button", { name: "Unlock" }).click();

  await expect(page.getByRole("alert")).toContainText("invalid token");
});

test("responsive portal layout", async ({ page }) => {
  await mockApi(page, { profiles: [profile], keys: [key] });
  await openPortal(page);

  await expect(page.getByRole("heading", { name: "Dense-Mem Control" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Profiles" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "API keys" })).toBeVisible();

  if ((page.viewportSize()?.width ?? 1000) < 700) {
    await expect(page.locator(".workspace")).toHaveCSS("grid-template-columns", /[0-9.]+px/);
  }
});

async function openPortal(page: Page) {
  await page.goto("/");
  await page.getByLabel("Control token").fill("secret");
  await page.getByRole("button", { name: "Unlock" }).click();
  await expect(page.getByRole("heading", { name: "Profiles" })).toBeVisible();
}

async function mockApi(page: Page, state: { profiles: TestProfile[]; keys: TestKey[] }) {
  let profiles = [...state.profiles];
  let keys = [...state.keys];
  await page.route("**/control/api/**", async (route) => {
    const url = route.request().url();
    const method = route.request().method();

    if (url.endsWith("/session")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { authenticated: true } }) });
    }
    if (url.endsWith("/profiles") && method === "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(pageOf(profiles)) });
    }
    if (url.endsWith("/profiles") && method === "POST") {
      const body = route.request().postDataJSON() as { name: string; description: string };
      const created = { ...profile, id: "33333333-3333-4333-8333-333333333333", name: body.name, description: body.description };
      profiles = [...profiles, created];
      return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ data: created }) });
    }
    if (url.includes("/api-keys") && method === "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(pageOf(keys)) });
    }
    if (url.includes("/api-keys") && method === "POST") {
      const body = route.request().postDataJSON() as { label?: string };
      expect(body.label).toBeUndefined();
      keys = [key, ...keys];
      return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ data: { api_key: "dm_plain_once", key } }) });
    }
    if (url.includes("/api-keys") && method === "DELETE") {
      keys = keys.filter((item) => !url.endsWith(`/api-keys/${item.id}`));
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { status: "deleted" } }) });
    }
    if (url.includes("/profiles/") && method === "DELETE") {
      profiles = [];
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { status: "deleted" } }) });
    }
    return route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ message: "not found" }) });
  });
}

function pageOf<T>(data: T[]) {
  return { data, pagination: { limit: 20, offset: 0, total: data.length } };
}
