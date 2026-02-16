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

		// If redirected to login, user already exists - skip onboarding
		if (currentUrl.includes("/login")) {
			helpers.log("User already exists - skipping onboarding");
			helpers.log(`Using existing credentials: ${TEST_EMAIL}`);
			return;
		}

		// Check if we're on the onboarding page (shows "Create your account")
		const heading = page.locator("h1");
		await expect(heading).toBeVisible({ timeout: 10000 });
		const headingText = await heading.textContent();
		helpers.log(`Page heading: ${headingText}`);

		// Step 1: Enter email
		const emailInput = page.locator('input[type="email"], input[name="email"], input#email');
		await expect(emailInput).toBeVisible({ timeout: 5000 });
		await emailInput.fill(TEST_EMAIL);
		helpers.log(`Email entered: ${TEST_EMAIL}`);

		// Submit email form
		await page.click('button[type="submit"]');
		await page.waitForLoadState("networkidle");

		// Step 2: Enter password
		const passwordInput = page.locator('input[name="password"], input#password');
		await expect(passwordInput).toBeVisible({ timeout: 10000 });
		await passwordInput.fill(TEST_PASSWORD);

		const confirmInput = page.locator('input[name="confirm_password"], input#confirm_password');
		await expect(confirmInput).toBeVisible();
		await confirmInput.fill(TEST_PASSWORD);
		helpers.log("Password entered");

		// Submit password form
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
