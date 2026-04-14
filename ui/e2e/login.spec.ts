/**
 * Login page tests.
 *
 * These run against a live Next.js dev server but mock the Argus API so
 * no real OAuth credentials or Postgres instance are needed.
 */

import { test, expect } from "@playwright/test";

test.describe("Login page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/login");
  });

  test("renders brand, heading and both OAuth buttons", async ({ page }) => {
    await expect(page).toHaveTitle(/Sign in — Argus/);

    // Heading
    await expect(
      page.getByRole("heading", { name: "Sign in to Argus" }),
    ).toBeVisible();

    // Both buttons
    const github = page.getByTestId("github-login");
    const google = page.getByTestId("google-login");
    await expect(github).toBeVisible();
    await expect(google).toBeVisible();

    // Labels
    await expect(github).toContainText("Continue with GitHub");
    await expect(google).toContainText("Continue with Google");
  });

  test("GitHub button links to the correct server auth endpoint", async ({
    page,
  }) => {
    const github = page.getByTestId("github-login");
    const href = await github.getAttribute("href");
    expect(href).toContain("/auth/github");
  });

  test("Google button links to the correct server auth endpoint", async ({
    page,
  }) => {
    const google = page.getByTestId("google-login");
    const href = await google.getAttribute("href");
    expect(href).toContain("/auth/google");
  });

  test("shows privacy note about signals only", async ({ page }) => {
    await expect(page.getByText("Signals only.")).toBeVisible();
  });
});
