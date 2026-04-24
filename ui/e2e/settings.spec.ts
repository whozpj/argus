/**
 * Settings page tests.
 * All API calls are intercepted — no live server needed.
 */

import { test, expect } from "@playwright/test";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

const ME_NO_NAME: object = {
  id: "user-1",
  email: "alice@example.com",
  display_name: null,
  projects: [{ id: "proj-1", name: "production", created_at: "2026-04-13T00:00:00Z" }],
};

const ME_WITH_NAME: object = {
  id: "user-1",
  email: "alice@example.com",
  display_name: "Alice",
  projects: [{ id: "proj-1", name: "production", created_at: "2026-04-13T00:00:00Z" }],
};

async function setToken(page: import("@playwright/test").Page) {
  await page.addInitScript(() => localStorage.setItem("argus_token", "mock-jwt"));
}

async function mockMe(
  page: import("@playwright/test").Page,
  me: object = ME_NO_NAME,
) {
  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(me) });
    } else {
      await route.continue();
    }
  });
}

// ── Redirect when not logged in ────────────────────────────────────────────────

test("settings redirects to /login when no token", async ({ page }) => {
  await page.goto("/settings");
  await page.waitForURL("/login", { timeout: 5000 });
  await expect(page).toHaveURL("/login");
});

// ── Page renders ───────────────────────────────────────────────────────────────

test.describe("Settings page — no display name set", () => {
  test.beforeEach(async ({ page }) => {
    await setToken(page);
    await mockMe(page, ME_NO_NAME);
    await page.goto("/settings");
    await page.waitForSelector("[data-testid='settings-form']", { timeout: 6000 });
  });

  test("renders display name input (empty)", async ({ page }) => {
    const input = page.getByTestId("display-name-input");
    await expect(input).toBeVisible();
    await expect(input).toHaveValue("");
  });

  test("renders account email read-only", async ({ page }) => {
    await expect(page.getByTestId("account-email")).toContainText("alice@example.com");
  });

  test("renders Save button", async ({ page }) => {
    await expect(page.getByTestId("save-button")).toBeVisible();
  });
});

test.describe("Settings page — display name already set", () => {
  test.beforeEach(async ({ page }) => {
    await setToken(page);
    await mockMe(page, ME_WITH_NAME);
    await page.goto("/settings");
    await page.waitForSelector("[data-testid='settings-form']", { timeout: 6000 });
  });

  test("pre-fills the input with existing display name", async ({ page }) => {
    await expect(page.getByTestId("display-name-input")).toHaveValue("Alice");
  });
});

// ── Save flow ──────────────────────────────────────────────────────────────────

test("successful save shows 'Saved' confirmation", async ({ page }) => {
  await setToken(page);
  await mockMe(page, ME_NO_NAME);

  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    if (route.request().method() === "PATCH") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ...ME_WITH_NAME }),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(ME_NO_NAME),
      });
    }
  });

  await page.goto("/settings");
  await page.waitForSelector("[data-testid='settings-form']", { timeout: 6000 });

  await page.getByTestId("display-name-input").fill("Alice");
  await page.getByTestId("save-button").click();

  await expect(page.getByTestId("save-success")).toBeVisible({ timeout: 4000 });
});

test("server error shows error message", async ({ page }) => {
  await setToken(page);
  await mockMe(page, ME_NO_NAME);

  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    if (route.request().method() === "PATCH") {
      await route.fulfill({ status: 400, body: "display_name must be 50 characters or fewer" });
    } else {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(ME_NO_NAME),
      });
    }
  });

  await page.goto("/settings");
  await page.waitForSelector("[data-testid='settings-form']", { timeout: 6000 });

  await page.getByTestId("display-name-input").fill("x");
  await page.getByTestId("save-button").click();

  await expect(page.getByTestId("save-error")).toBeVisible({ timeout: 4000 });
});

// ── Dashboard header shows display name ───────────────────────────────────────

test("dashboard header shows display_name when set", async ({ page }) => {
  await setToken(page);

  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(ME_WITH_NAME),
    });
  });
  await page.route(`${SERVER}/api/v1/baselines**`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ total_events: 0, baselines: [] }),
    });
  });

  await page.goto("/dashboard");
  await page.waitForSelector("[data-testid='user-email']", { timeout: 8000 });
  await expect(page.getByTestId("user-email")).toContainText("Alice");
});

test("dashboard header falls back to email when display_name is null", async ({ page }) => {
  await setToken(page);

  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(ME_NO_NAME),
    });
  });
  await page.route(`${SERVER}/api/v1/baselines**`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ total_events: 0, baselines: [] }),
    });
  });

  await page.goto("/dashboard");
  await page.waitForSelector("[data-testid='user-email']", { timeout: 8000 });
  await expect(page.getByTestId("user-email")).toContainText("alice@example.com");
});
