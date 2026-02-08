// e2e/001-onboarding.spec.js - MUST RUN FIRST to set up user account
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

// Global test credentials - shared across all test files
const TEST_EMAIL = "admin@test-e2e.com";
const TEST_PASSWORD = "testpassword123";

test.describe("Onboarding Flow - MUST RUN FIRST", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		helpers.log("=== ONBOARDING SETUP (Pro) - Creating User Account ===");

		// Capture console logs from the browser
		page.on('console', msg => {
			if (msg.text().includes('===')) {
				console.log(`[BROWSER] ${msg.text()}`);
			}
		});
	});

	test.afterEach(async ({ page }) => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("1. Complete Onboarding Flow - Creates user account for all other tests", async ({ page }) => {
		helpers.log("=== PHASE 1: ONBOARDING SETUP (Pro Version) ===");

		// Clear any existing data
		await page.context().clearCookies();
		await page.context().clearPermissions();

		// Step 1.1: Start onboarding
		await helpers.navigateTo("/setup", {
			waitForSelector: 'input[name="email"]',
			timeout: 30000
		});

		// Verify we're on setup page
		const pageContent = await page.textContent('body');
		expect(pageContent).toContain('Initial Setup');
		helpers.log("Setup page loaded");

		// Step 1.2: User account setup (email-based)
		await helpers.fillForm({
			email: TEST_EMAIL
		});

		// Click Continue button
		await page.click('button[type="submit"]');
		await page.waitForTimeout(2000);
		helpers.log(`User account configured with email: ${TEST_EMAIL}`);

		// Step 1.3: Password setup
		await page.waitForSelector('input[name="password"]', { timeout: 10000 });
		// Use submitButton: null to prevent fillForm from auto-submitting
		// We'll submit manually to have more control
		await helpers.fillForm({
			password: TEST_PASSWORD,
			confirm_password: TEST_PASSWORD
		}, { submitButton: null });

		// Click Continue button - wait for GeoLite step to load after
		const passwordSubmit = page.locator('button[type="submit"]');
		await passwordSubmit.waitFor({ state: 'visible', timeout: 10000 });
		await passwordSubmit.click();
		// Wait for step transition before continuing
		await page.waitForSelector('text=GeoLite2 Database', { timeout: 10000 });
		helpers.log("Password configured");

		// Step 1.4: GeoLite setup (skip for now)
		helpers.log("GeoLite step loaded");

		// Wait for skip button to be ready and click it
		const geoLiteSkipBtn = page.locator('button:has-text("Skip for now")');
		await geoLiteSkipBtn.waitFor({ state: 'visible', timeout: 10000 });
		await expect(geoLiteSkipBtn).toBeEnabled({ timeout: 5000 });
		await geoLiteSkipBtn.click();
		helpers.log("GeoLite step skipped");

		// Step 1.5: License key setup - Pro only
		// Wait for page to fully transition to License step
		await page.waitForTimeout(500);
		await page.waitForSelector('text=License Key', { timeout: 10000 });
		helpers.log("License step loaded");

		// Enter the sandbox license key (this step is required for Pro)
		await page.fill('input[name="license_key"]', '1E837996-DEC5-4930-8B38-D7F6648E33DB');

		// Submit the license key
		const licenseSubmitBtn = page.locator('button[type="submit"]');
		await licenseSubmitBtn.waitFor({ state: 'visible', timeout: 10000 });
		await expect(licenseSubmitBtn).toBeEnabled({ timeout: 5000 });
		await licenseSubmitBtn.click();
		helpers.log("License key submitted");

		// Step 1.6: OpenAI setup (skip for now) - Pro only
		// Wait for page to fully transition to OpenAI step
		await page.waitForTimeout(500);
		await page.waitForSelector('text=AI-Powered Analytics', { timeout: 10000 });
		helpers.log("OpenAI step loaded");

		// Wait for network to settle and loading state to update
		await page.waitForLoadState('networkidle');

		// Wait for the OpenAI skip button to be visible and enabled
		const openAISkipBtn = page.locator('button:has-text("Skip for now")');
		await openAISkipBtn.waitFor({ state: 'visible', timeout: 10000 });
		await expect(openAISkipBtn).toBeEnabled({ timeout: 10000 });
		await openAISkipBtn.click();
		helpers.log("OpenAI step skipped");

		// Final redirect check - should be logged in
		await page.waitForURL(/\/admin\/websites\/new/, { timeout: 15000 });
		const finalUrl = page.url();
		expect(finalUrl).not.toContain("/setup");
		expect(finalUrl).not.toContain("/login");

		helpers.log(`USER ACCOUNT CREATED: ${TEST_EMAIL} / ${TEST_PASSWORD}`);
		helpers.log(`Final URL: ${finalUrl}`);
		helpers.log("Onboarding completed - all other tests can now use this account");
	});
});
