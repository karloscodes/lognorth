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

	test("Home page shows correct heading", async ({ page }) => {
		helpers.log("Testing home page");

		await helpers.navigateTo("/");

		// When no apps: redirects to /apps showing "Add your first app"
		// When apps exist: shows "Overview"
		const h1 = page.locator("h1");
		await expect(h1).toBeVisible({ timeout: 10000 });
		const text = await h1.textContent();
		expect(text === "Add your first app" || text === "Overview").toBeTruthy();
		helpers.log(`Home page loaded with heading: ${text}`);
	});

	test("Can access Settings page", async ({ page }) => {
		helpers.log("Testing settings page");

		await helpers.navigateTo("/settings");

		await expect(page.locator("h1")).toContainText("Notifications");
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
