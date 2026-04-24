/**
 * Dashboard tests.
 *
 * All API calls are intercepted via page.route() — no live server needed.
 */

import { test, expect } from "@playwright/test";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

const ME_RESPONSE = {
  id: "user-1",
  email: "alice@example.com",
  projects: [
    { id: "proj-1", name: "production", created_at: "2026-04-13T00:00:00Z" },
    { id: "proj-2", name: "staging", created_at: "2026-04-13T00:00:00Z" },
  ],
};

const BASELINES_EMPTY = { total_events: 0, baselines: [] };

const BASELINES_WITH_DATA = {
  total_events: 5432,
  baselines: [
    {
      model: "claude-sonnet-4-6",
      count: 300,
      mean_output_tokens: 82.5,
      stddev_output_tokens: 12.3,
      mean_latency_ms: 843.0,
      stddev_latency_ms: 67.0,
      is_ready: true,
      drift_score: 0.0,
      drift_alerted: false,
      p_output_tokens: 0.0,
      p_latency_ms: 0.0,
    },
    {
      model: "gpt-4o",
      count: 120,
      mean_output_tokens: 94.0,
      stddev_output_tokens: 8.0,
      mean_latency_ms: 1200.0,
      stddev_latency_ms: 120.0,
      is_ready: false,
      drift_score: 0.0,
      drift_alerted: false,
      p_output_tokens: 0.0,
      p_latency_ms: 0.0,
    },
  ],
};

const BASELINES_WITH_DRIFT = {
  total_events: 8000,
  baselines: [
    {
      model: "claude-sonnet-4-6",
      count: 400,
      mean_output_tokens: 110.0,
      stddev_output_tokens: 20.0,
      mean_latency_ms: 1100.0,
      stddev_latency_ms: 90.0,
      is_ready: true,
      drift_score: 0.82,
      drift_alerted: true,
      p_output_tokens: 0.001,
      p_latency_ms: 0.0008,
    },
  ],
};

// Helper: inject a token into localStorage before the page loads.
async function setToken(page: import("@playwright/test").Page, token = "mock-jwt") {
  await page.addInitScript((t) => {
    localStorage.setItem("argus_token", t);
  }, token);
}

// Helper: wire all required API routes and wait for the dashboard to be ready.
async function mockAPIs(
  page: import("@playwright/test").Page,
  {
    me = ME_RESPONSE,
    baselines = BASELINES_EMPTY,
    meStatus = 200,
  }: { me?: object; baselines?: object; meStatus?: number } = {},
) {
  await page.route(`${SERVER}/api/v1/me`, async (route) => {
    await route.fulfill({
      status: meStatus,
      contentType: "application/json",
      body: meStatus === 200 ? JSON.stringify(me) : "unauthorized",
    });
  });
  await page.route(`${SERVER}/api/v1/baselines**`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(baselines),
    });
  });
}

/** Wait until the dashboard is fully rendered (not just mounted). */
async function waitForDashboard(page: import("@playwright/test").Page) {
  await page.waitForSelector("[data-testid='dashboard']", { timeout: 8000 });
  // Also wait for the user email — indicates /api/v1/me has resolved.
  await page.waitForSelector("[data-testid='user-email']", { timeout: 8000 });
}

// ── Unauthenticated ────────────────────────────────────────────────────────────

test.describe("Dashboard — unauthenticated", () => {
  test("redirects to /login when no token in localStorage", async ({ page }) => {
    await page.goto("/dashboard");
    await page.waitForURL("/login", { timeout: 6000 });
    await expect(page).toHaveURL("/login");
  });
});

// ── Authenticated, no data ─────────────────────────────────────────────────────

test.describe("Dashboard — authenticated, no data", () => {
  test.beforeEach(async ({ page }) => {
    await setToken(page);
    await mockAPIs(page, { baselines: BASELINES_EMPTY });
    await page.goto("/dashboard");
    await waitForDashboard(page);
  });

  test("shows user email in header", async ({ page }) => {
    await expect(page.getByTestId("user-email")).toContainText("alice@example.com");
  });

  test("shows project selector with the first project selected", async ({ page }) => {
    const selector = page.getByTestId("project-selector");
    await expect(selector).toBeVisible();
    // Wait for the project name to appear (auto-selection triggers a re-render).
    await expect(selector).toContainText("production", { timeout: 6000 });
  });

  test("shows sign-out option in user dropdown", async ({ page }) => {
    // Sign out is now inside the dropdown — open it first.
    await page.getByTestId("user-email").click();
    await expect(page.getByTestId("sign-out")).toBeVisible({ timeout: 3000 });
  });

  test("shows empty state when no events", async ({ page }) => {
    // Wait for loading skeleton to disappear.
    await expect(page.getByText("No events yet")).toBeVisible({ timeout: 6000 });
  });

  test("shows summary cards", async ({ page }) => {
    // Wait past skeleton.
    await expect(page.getByText("No events yet")).toBeVisible({ timeout: 6000 });
    await expect(page.getByText("Total Events")).toBeVisible();
    await expect(page.getByText("Models").first()).toBeVisible();
  });
});

// ── Authenticated, with data ───────────────────────────────────────────────────

test.describe("Dashboard — authenticated, with data", () => {
  test.beforeEach(async ({ page }) => {
    await setToken(page);
    await mockAPIs(page, { baselines: BASELINES_WITH_DATA });
    await page.goto("/dashboard");
    await waitForDashboard(page);
    // Ensure baselines table is rendered before assertions.
    await page.waitForSelector("tbody tr", { timeout: 6000 });
  });

  test("shows total events count", async ({ page }) => {
    await expect(page.getByText("5,432")).toBeVisible();
  });

  test("renders model rows in the table", async ({ page }) => {
    await expect(page.getByText("claude-sonnet-4-6")).toBeVisible();
    await expect(page.getByText("gpt-4o")).toBeVisible();
  });

  test("shows Ready badge for baseline-ready models", async ({ page }) => {
    const rows = page.locator("tbody tr");
    // First row (claude-sonnet-4-6, is_ready=true) — Ready badge in Baseline col.
    await expect(rows.first().locator("[data-slot='badge']").filter({ hasText: "Ready" })).toBeVisible();
  });

  test("shows warming-up count badge for non-ready models", async ({ page }) => {
    // gpt-4o has count=120, not ready — shows "120/200" badge in Status column.
    await expect(
      page.locator("tbody").getByText("120/200").first(),
    ).toBeVisible();
  });

  test("no drift alert shown when score is 0", async ({ page }) => {
    await expect(page.getByTestId("drift-alert")).not.toBeVisible();
  });
});

// ── Drift active ───────────────────────────────────────────────────────────────

test.describe("Dashboard — drift active", () => {
  test.beforeEach(async ({ page }) => {
    await setToken(page);
    await mockAPIs(page, { baselines: BASELINES_WITH_DRIFT });
    await page.goto("/dashboard");
    await waitForDashboard(page);
    await page.waitForSelector("[data-testid='drift-alert']", { timeout: 6000 });
  });

  test("shows drift alert banner with model details", async ({ page }) => {
    const alert = page.getByTestId("drift-alert");
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("claude-sonnet-4-6");
    await expect(alert).toContainText("0.82");
  });

  test("shows Drift badge in the table row", async ({ page }) => {
    // Use the tbody scope to avoid matching header/banner text.
    await expect(
      page.locator("tbody").getByText("Drift"),
    ).toBeVisible({ timeout: 6000 });
  });

  test("Alerts stat shows count of 1", async ({ page }) => {
    // The number "1" under the Alerts label.
    await expect(page.getByText("Alerts").locator("..").locator("..").getByText("1")).toBeVisible();
  });
});

// ── Sign out ───────────────────────────────────────────────────────────────────

test.describe("Dashboard — sign out", () => {
  test("clears token and redirects to /login on sign-out", async ({ page }) => {
    await setToken(page);
    await mockAPIs(page);
    await page.goto("/dashboard");
    await waitForDashboard(page);

    // Open the user dropdown first, then click sign out.
    await page.getByTestId("user-email").click();
    await page.getByTestId("sign-out").click();
    await page.waitForURL("/login", { timeout: 5000 });

    const token = await page.evaluate(() => localStorage.getItem("argus_token"));
    expect(token).toBeNull();
  });
});

// ── Refresh button ─────────────────────────────────────────────────────────────

test.describe("Dashboard — refresh button", () => {
  test("refresh button triggers a new baselines fetch", async ({ page }) => {
    let callCount = 0;
    await setToken(page);

    await page.route(`${SERVER}/api/v1/me`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(ME_RESPONSE),
      });
    });
    await page.route(`${SERVER}/api/v1/baselines**`, async (route) => {
      callCount++;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(BASELINES_EMPTY),
      });
    });

    await page.goto("/dashboard");
    await waitForDashboard(page);
    await page.getByText("No events yet").waitFor({ timeout: 6000 });
    const beforeCount = callCount;

    await page.getByRole("button", { name: "Refresh" }).click();
    // Wait briefly for the new fetch to fire.
    await page.waitForTimeout(600);
    expect(callCount).toBeGreaterThan(beforeCount);
  });
});
