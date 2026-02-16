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

	test("Home page shows overview", async ({ page }) => {
		helpers.log("Testing home page");

		await helpers.navigateTo("/");

		// Should show Overview heading
		await expect(page.locator("h1")).toContainText("Overview");
		helpers.log("Home page loaded with Overview heading");
	});

	test("Can access Settings page", async ({ page }) => {
		helpers.log("Testing settings page");

		await helpers.navigateTo("/settings");

		// Settings page has "Notifications" as the main heading
		await expect(page.locator("h1")).toContainText("Notifications");
		helpers.log("Settings page loaded");
	});

	test("Can access Apps management", async ({ page }) => {
		helpers.log("Testing apps page");

		await helpers.navigateTo("/apps");

		await expect(page.locator("h1")).toContainText("Apps");
		helpers.log("Apps page loaded");
	});
});
