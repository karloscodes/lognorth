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

		// Wait for React to hydrate - look for actual page content
		await page.waitForSelector('input, h1', { timeout: 15000 });
		await page.waitForLoadState("networkidle");

		const currentUrl = page.url();

		// /setup must show the onboarding form on a fresh test DB. If it
		// redirected to /login, the server is holding stale state from a
		// previous run (e.g. dev server left running across the DB wipe in
		// setup-test-env.js). Fail loudly — silently skipping leaves no
		// user behind and breaks every test that follows.
		if (currentUrl.includes("/login")) {
			throw new Error(
				`/setup redirected to /login on a freshly wiped DB — server is in stale state. ` +
				`Make sure setup-test-env.js killed the previous server, then re-run.`
			);
		}

		// Check if we're on the onboarding page (shows "Create your account")
		const heading = page.locator("h1");
		await expect(heading).toBeVisible({ timeout: 10000 });
		const headingText = await heading.textContent();
		helpers.log(`Page heading: ${headingText}`);

		// One form: email, password, and confirmation
		const emailInput = page.locator('input[type="email"], input[name="email"], input#email');
		await expect(emailInput).toBeVisible({ timeout: 5000 });
		await emailInput.fill(TEST_EMAIL);
		helpers.log(`Email entered: ${TEST_EMAIL}`);

		await page.locator('input[name="password"], input#password').fill(TEST_PASSWORD);
		await page.locator('input[name="confirm_password"], input#confirm_password').fill(TEST_PASSWORD);
		helpers.log("Password entered");

		// Submit the form
		await page.click('button[type="submit"]');
		await page.waitForLoadState("networkidle");

		// Should be logged in and redirected
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
