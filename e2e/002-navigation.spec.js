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

		await helpers.navigateTo("/");

		// When no apps: redirects to /apps showing "Add your first app"
		// When apps exist: shows issue count or "All clear"
		const h1 = page.locator("h1");
		const h1Visible = await h1.isVisible({ timeout: 5000 }).catch(() => false);

		if (h1Visible) {
			// No apps yet - redirected to /apps
			const text = await h1.textContent();
			expect(text).toBe("Add your first app");
			helpers.log("Home page: no apps, redirected to apps page");
		} else {
			// Apps exist - check for issue count or "All clear"
			const issueCount = page.locator("text=/\\d+ issues?/");
			const allClear = page.locator("text=All clear");
			const hasIssues = await issueCount.isVisible({ timeout: 5000 }).catch(() => false);
			const hasAllClear = await allClear.isVisible({ timeout: 5000 }).catch(() => false);
			expect(hasIssues || hasAllClear).toBeTruthy();
			helpers.log(`Home page: ${hasIssues ? "has issues" : "all clear"}`);
		}
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
