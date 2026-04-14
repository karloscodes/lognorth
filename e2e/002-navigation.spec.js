// e2e/002-navigation.spec.js - Main navigation tests
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

test.describe("Navigation Tests", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		await helpers.login();
	});

	test.afterEach(async () => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("Home page shows correct content", async ({ page }) => {
		helpers.log("Testing home page");

		// First create an app if none exists
		await helpers.navigateTo("/apps");
		const nameInput = page.locator('input[name="name"]');
		if (await nameInput.isVisible({ timeout: 2000 }).catch(() => false)) {
			helpers.log("No apps yet, creating first app");
			await nameInput.fill("Navigation Test App");
			await page.click('button[type="submit"]');
			await page.waitForLoadState("networkidle");
			helpers.log("App created");
		}

		// Now test home page
		await helpers.navigateTo("/");

		// With apps: shows issue count or "All clear"
		const issueCount = page.locator("text=/\\d+ issues?/");
		const allClear = page.locator("text=All clear");
		const hasIssues = await issueCount.isVisible({ timeout: 5000 }).catch(() => false);
		const hasAllClear = await allClear.isVisible({ timeout: 5000 }).catch(() => false);
		expect(hasIssues || hasAllClear).toBeTruthy();
		helpers.log(`Home page: ${hasIssues ? "has issues" : "all clear"}`);
	});

	test("Can access Settings page", async ({ page }) => {
		helpers.log("Testing settings page");

		await helpers.navigateTo("/settings");

		// Settings page has multiple section headings (License, Notifications, ...).
		// Asserting the first h1 fails when sections get reordered, so check that
		// the Notifications section is rendered somewhere.
		await expect(page.locator("h1", { hasText: "Notifications" })).toBeVisible();
		helpers.log("Settings page loaded");
	});

	test("Can access Apps management", async ({ page }) => {
		helpers.log("Testing apps page");

		await helpers.navigateTo("/apps");

		// Shows "Add your first app" when no apps exist, "Apps" when apps exist
		const h1 = page.locator("h1");
		await expect(h1).toBeVisible({ timeout: 10000 });
		const text = await h1.textContent();
		expect(text === "Add your first app" || text === "Apps").toBeTruthy();
		helpers.log(`Apps page loaded with heading: ${text}`);
	});
});
