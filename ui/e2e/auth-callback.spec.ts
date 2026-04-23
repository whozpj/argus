/**
 * Auth callback page tests.
 *
 * Uses page.route() to intercept the token-exchange API call so we never
 * need real OAuth credentials.
 */

import { test, expect } from "@playwright/test";

const SERVER = process.env.NEXT_PUBLIC_ARGUS_SERVER ?? "http://localhost:4000";

test.describe("Auth callback page", () => {
  test("redirects to /login when no code param is present", async ({
    page,
  }) => {
    await page.goto("/auth/callback");
    await page.waitForURL("/login", { timeout: 5000 });
    await expect(page).toHaveURL("/login");
  });

  test("exchanges code for token and redirects to /dashboard on success", async ({
    page,
  }) => {
    // Mock the token-exchange endpoint.
    await page.route(`${SERVER}/api/v1/auth/token`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ token: "mock-jwt-token", email: "user@example.com" }),
      });
    });

    // Mock /api/v1/me so the dashboard doesn't immediately redirect back to /login.
    await page.route(`${SERVER}/api/v1/me`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "user-1",
          email: "user@example.com",
          projects: [{ id: "proj-1", name: "production", created_at: "2026-04-13T00:00:00Z" }],
        }),
      });
    });

    // Mock /api/v1/baselines for the dashboard.
    await page.route(`${SERVER}/api/v1/baselines**`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ total_events: 0, baselines: [] }),
      });
    });

    await page.goto("/auth/callback?code=valid-code-123");

    await page.waitForURL(/\/dashboard/, { timeout: 8000 });
    expect(page.url()).toContain("/dashboard");

    // Token must be stored in localStorage.
    const token = await page.evaluate(() => localStorage.getItem("argus_token"));
    expect(token).toBe("mock-jwt-token");
  });

  test("redirects to /login when token exchange fails", async ({ page }) => {
    await page.route(`${SERVER}/api/v1/auth/token`, async (route) => {
      await route.fulfill({ status: 401 });
    });

    await page.goto("/auth/callback?code=bad-code");
    await page.waitForURL("/login", { timeout: 6000 });
    await expect(page).toHaveURL("/login");
  });

  test("shows signing-in spinner while loading", async ({ page }) => {
    // Delay the response so we can observe the loading state.
    await page.route(`${SERVER}/api/v1/auth/token`, async (route) => {
      await new Promise((r) => setTimeout(r, 1500));
      await route.fulfill({ status: 401 });
    });

    await page.goto("/auth/callback?code=slow-code");
    await expect(page.getByText("Signing you in…")).toBeVisible();
  });
});
