// e2e/001-onboarding.spec.js - MUST RUN FIRST to set up user account
const { test, expect } = require("@playwright/test");
const { TestHelpers, TEST_EMAIL, TEST_PASSWORD } = require("./test-helpers");

test.describe("Onboarding Flow - MUST RUN FIRST", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		helpers.log("=== ONBOARDING SETUP ===");
	});

	test.afterEach(async () => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("1. Complete Onboarding - Creates user account for all other tests", async ({ page }) => {
		helpers.log("Starting onboarding flow");

		// Clear cookies
		await page.context().clearCookies();

		// Try to navigate to setup - may redirect to login if user exists
		await page.goto("/setup");
		await page.waitForLoadState("networkidle");

		const currentUrl = page.url();

		// If redirected to login, user already exists (seeded data) - skip onboarding
		if (currentUrl.includes("/login")) {
			helpers.log("User already exists (seeded data) - skipping onboarding");
			helpers.log(`Using existing credentials: ${TEST_EMAIL}`);
			return;
		}

		// Verify we're on setup page
		const pageContent = await page.textContent("body");
		expect(pageContent).toContain("Setup");
		helpers.log("Setup page loaded");

		// Step 1: Enter email
		await helpers.fillForm({ email: TEST_EMAIL }, { submitButton: null });
		await page.click('button[type="submit"]');
		await page.waitForLoadState("networkidle");
		helpers.log(`Email entered: ${TEST_EMAIL}`);

		// Step 2: Enter password
		await page.waitForSelector('input[name="password"]', { timeout: 10000 });
		await helpers.fillForm(
			{
				password: TEST_PASSWORD,
				confirm_password: TEST_PASSWORD
			},
			{ submitButton: null }
		);

		// Submit password form
		await page.click('button[type="submit"]');
		await page.waitForLoadState("networkidle");
		helpers.log("Password configured");

		// Should be logged in and redirected to home
		await page.waitForURL(url => !url.href.includes("/setup") && !url.href.includes("/login"), {
			timeout: 15000
		});

		const finalUrl = page.url();
		expect(finalUrl).not.toContain("/setup");
		expect(finalUrl).not.toContain("/login");

		helpers.log(`USER ACCOUNT CREATED: ${TEST_EMAIL}`);
		helpers.log(`Final URL: ${finalUrl}`);
		helpers.log("Onboarding completed - all other tests can now use this account");
	});
});
