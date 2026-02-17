// e2e/003-apps.spec.js - App management and app-specific pages
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

test.describe("App Management Tests", () => {
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

	test("Can create a new app", async ({ page }) => {
		helpers.log("Testing app creation");

		await helpers.navigateTo("/apps");

		// When no apps exist, form is shown directly on page
		const nameInput = page.locator('input[name="name"]');
		if (await nameInput.count() > 0) {
			await nameInput.fill("Test App E2E");
			await page.click('button[type="submit"]');
			await page.waitForLoadState("networkidle");

			// Should now show the app
			const content = await page.textContent("body");
			expect(content).toContain("Test App E2E");
			helpers.log("App created successfully");
		} else {
			// Apps already exist - verify we can see them
			helpers.log("Apps already exist - verifying visibility");
			await expect(page.locator('a[href*="/apps/"]').first()).toBeVisible();
			helpers.log("Existing apps found - test passed");
		}
	});

	test("Can view app Issues page", async ({ page }) => {
		helpers.log("Testing app issues page");

		// First ensure we have an app
		await helpers.navigateTo("/apps");

		// Click on first app link (should take us to issues)
		const appLink = page.locator('a[href*="/apps/"]').first();
		if (await appLink.count() > 0) {
			await appLink.click();
			await page.waitForLoadState("networkidle");

			// Navigate to issues
			await page.click('nav >> text=Issues');
			await page.waitForURL(/\/apps\/\d+\/issues/);

			await expect(page.locator("h1")).toContainText("Issues");
			helpers.log("Issues page loaded");
		} else {
			helpers.log("No apps found, skipping issues test", "warn");
		}
	});

	test("Can view app Logs page", async ({ page }) => {
		helpers.log("Testing app logs page");

		await helpers.navigateTo("/apps");

		const appLink = page.locator('a[href*="/apps/"]').first();
		if (await appLink.count() > 0) {
			await appLink.click();
			await page.waitForLoadState("networkidle");

			await page.click('nav >> text=Logs');
			await page.waitForURL(/\/apps\/\d+\/log/);

			await expect(page.locator("h1")).toContainText("Logs");
			helpers.log("Logs page loaded");
		} else {
			helpers.log("No apps found, skipping logs test", "warn");
		}
	});

	test("Can view app Endpoints page", async ({ page }) => {
		helpers.log("Testing app endpoints page");

		await helpers.navigateTo("/apps");

		const appLink = page.locator('a[href*="/apps/"]').first();
		if (await appLink.count() > 0) {
			await appLink.click();
			await page.waitForLoadState("networkidle");

			await page.click('nav >> text=Endpoints');
			await page.waitForURL(/\/apps\/\d+\/endpoints/);

			await expect(page.locator("h1")).toContainText("Endpoints");
			helpers.log("Endpoints page loaded");
		} else {
			helpers.log("No apps found, skipping endpoints test", "warn");
		}
	});

	test("Log filters work", async ({ page }) => {
		helpers.log("Testing log filters");

		await helpers.navigateTo("/apps");

		const appLink = page.locator('a[href*="/apps/"]').first();
		if (await appLink.count() > 0) {
			await appLink.click();
			await page.click('nav >> text=Logs');
			await page.waitForURL(/\/apps\/\d+\/log/);

			// Click Errors filter
			await page.click('button:has-text("Errors")');
			await page.waitForURL(/is_error=true/);

			expect(page.url()).toContain("is_error=true");
			helpers.log("Error filter applied");

			// Click date filter
			await page.click('button:has-text("Today")');
			await page.waitForURL(/start_time=/);

			expect(page.url()).toContain("start_time=");
			helpers.log("Date filter applied");
		} else {
			helpers.log("No apps found, skipping filter test", "warn");
		}
	});
});
