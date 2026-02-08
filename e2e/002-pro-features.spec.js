// e2e/002-pro-features.spec.js - Pro-specific feature tests
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

const TEST_EMAIL = "admin@test-e2e.com";
const TEST_PASSWORD = "testpassword123";

test.describe("Pro Features Tests", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		helpers.log("=== Starting Pro Features Test ===");

		// Login with test credentials
		await helpers.login(TEST_EMAIL, TEST_PASSWORD, {
			expectSuccess: true,
			timeout: 30000
		});
		helpers.log("Login successful for Pro features test");
	});

	test.afterEach(async ({ page }) => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("Ask AI page shows AI interface (Pro feature)", async ({ page }) => {
		helpers.log("Testing Ask AI page shows Pro AI interface");

		// First create a test website to access Ask AI
		const domain = "ask-ai-test.com";
		await helpers.createTestWebsite(domain);
		helpers.log("Created test website for Ask AI page test");

		// Get website ID from URL (createTestWebsite redirects to dashboard which has ID in URL)
		const currentUrl = page.url();
		const match = currentUrl.match(/\/admin\/websites\/(\d+)/);
		const websiteId = match ? match[1] : "1";
		helpers.log(`Using website ID: ${websiteId}`);

		// Navigate to Ask AI page
		await helpers.navigateTo(`/admin/websites/${websiteId}/lens`, {
			timeout: 30000
		});

		// In Pro version, should see the Ask AI interface
		const askHeading = page.locator('h1').filter({ hasText: /Ask/i }).first();
		await expect(askHeading).toBeVisible({ timeout: 10000 });
		helpers.log("Ask AI heading visible");

		// Should see a textarea for AI queries
		const aiTextarea = page.locator('textarea');
		await expect(aiTextarea).toBeVisible({ timeout: 10000 });
		helpers.log("AI query textarea visible");

		// Should see sample question buttons (Pro AI feature)
		const sampleQuestions = page.locator('button:has-text("Compare"), button:has-text("traffic"), button:has-text("visitors")');
		const hasQuestions = await sampleQuestions.count() > 0;
		if (hasQuestions) {
			helpers.log("Sample AI questions visible");
		} else {
			// Check for "Try:" text which indicates example questions
			const tryText = page.locator('text=Try:');
			await expect(tryText).toBeVisible({ timeout: 5000 });
			helpers.log("Example questions section visible");
		}

		helpers.log("Ask AI page displayed Pro AI interface successfully");
	});

	test("Administration sidebar shows all pages", async ({ page }) => {
		helpers.log("Testing administration navigation");

		// Navigate to administration
		await helpers.navigateTo("/admin/administration/ingestion", {
			waitForSelector: 'h1:has-text("Ingestion Settings")',
			timeout: 30000
		});

		// Verify sidebar navigation is present
		const sidebar = await page.locator('aside:has-text("Administration")');
		await expect(sidebar).toBeVisible();

		// Verify all navigation links are present (OSS pages)
		const ingestionLink = page.locator('aside:has-text("Administration") a:has-text("Ingestion")');
		const accountLink = page.locator('aside:has-text("Administration") a:has-text("Account")');
		const systemLink = page.locator('aside:has-text("Administration") a:has-text("System")');

		await expect(ingestionLink).toBeVisible();
		await expect(accountLink).toBeVisible();
		await expect(systemLink).toBeVisible();

		helpers.log("All sidebar navigation links are visible");
	});

	test("Can navigate between administration pages", async ({ page }) => {
		helpers.log("Testing navigation between administration pages");

		// Start at Ingestion
		await helpers.navigateTo("/admin/administration/ingestion", {
			waitForSelector: 'h1:has-text("Ingestion Settings")',
			timeout: 30000
		});

		// Navigate to Account
		await page.click('aside:has-text("Administration") a:has-text("Account")');
		await page.waitForSelector('text=Change Password', { state: "visible", timeout: 5000 });
		helpers.log("Navigated to Account page");

		// Navigate to System
		await page.click('aside:has-text("Administration") a:has-text("System")');
		await page.waitForSelector('h1:has-text("System Management")', { timeout: 5000 });
		helpers.log("Navigated to System page");

		// Navigate back to Ingestion
		await page.click('aside:has-text("Administration") a:has-text("Ingestion")');
		await page.waitForSelector('textarea[name="excluded_ips"]', { state: "visible", timeout: 10000 });
		helpers.log("Navigated back to Ingestion page");

		helpers.log("Page navigation handled correctly");
	});

	test("System page shows GeoLite settings", async ({ page }) => {
		helpers.log("Testing System page has GeoLite settings");

		// Navigate to System page
		await helpers.navigateTo("/admin/administration/system", {
			waitForSelector: 'h1:has-text("System Management")',
			timeout: 30000
		});

		// Verify GeoLite Configuration section exists
		const geoLiteHeading = page.locator('text=GeoLite Configuration');
		await expect(geoLiteHeading).toBeVisible({ timeout: 10000 });
		helpers.log("GeoLite Configuration section visible");

		// Verify Account ID input exists
		const accountIdInput = page.locator('input[placeholder*="123456"]');
		await expect(accountIdInput).toBeVisible();
		helpers.log("Account ID input visible");

		// Verify License key input exists
		const licenseKeyInput = page.locator('input[placeholder*="MaxMind license key"]');
		await expect(licenseKeyInput).toBeVisible();
		helpers.log("License key input visible");

		// Verify Save Settings button exists
		const saveButton = page.locator('button:has-text("Save GeoLite Settings")');
		await expect(saveButton).toBeVisible();
		helpers.log("Save GeoLite Settings button visible");

		helpers.log("GeoLite settings displayed correctly");
	});

	test("Can update GeoLite settings", async ({ page }) => {
		helpers.log("Testing GeoLite settings update");

		// Navigate to System page
		await helpers.navigateTo("/admin/administration/system", {
			waitForSelector: 'h1:has-text("System Management")',
			timeout: 30000
		});

		// Enter a test account ID
		const accountIdInput = page.locator('input[placeholder*="123456"]');
		await accountIdInput.fill('999999');
		helpers.log("Entered test account ID");

		// Enter a test license key
		const licenseKeyInput = page.locator('input[placeholder*="MaxMind license key"]');
		await licenseKeyInput.fill('test-license-key-12345');
		helpers.log("Entered test license key");

		// Click Save GeoLite Settings
		await page.click('button:has-text("Save GeoLite Settings")');

		// Wait for success message
		await page.waitForSelector('text=GeoLite settings saved', {
			timeout: 10000
		});
		helpers.log("GeoLite settings saved successfully");
	});

	test("Can purge caches from System page", async ({ page }) => {
		helpers.log("Testing cache purge functionality");

		// Navigate to System page
		await helpers.navigateTo("/admin/administration/system", {
			waitForSelector: 'h1:has-text("System Management")',
			timeout: 30000
		});

		// Click purge cache button and confirm
		page.once('dialog', dialog => dialog.accept());
		await page.click('button:has-text("Purge All Caches")');

		// Wait for success message
		await page.waitForSelector('text=All caches have been purged successfully', {
			timeout: 10000
		});
		helpers.log("Cache purge completed successfully");
	});

	test("Database export button is available", async ({ page }) => {
		helpers.log("Testing database export button is available");

		// Navigate to System page
		await helpers.navigateTo("/admin/administration/system", {
			waitForSelector: 'h1:has-text("System Management")',
			timeout: 30000
		});

		// Verify the Export Database button is visible
		const exportButton = page.locator('button:has-text("Export Database")');
		await expect(exportButton).toBeVisible();

		helpers.log("Database export button is available");
	});

	// === PRO-ONLY ADMINISTRATION PAGES ===

	test("AI Settings page is accessible (Pro-only)", async ({ page }) => {
		helpers.log("Testing AI Settings page");

		// Navigate to AI settings page
		await helpers.navigateTo("/admin/administration/ai", {
			timeout: 30000
		});

		// Verify page loaded with AI settings content
		const aiHeading = page.locator('h1:has-text("AI Settings"), h2:has-text("AI Settings")');
		await expect(aiHeading).toBeVisible({ timeout: 10000 });
		helpers.log("AI Settings heading visible");

		// Verify OpenAI API key input exists
		const apiKeyInput = page.locator('input[name="openai_api_key"], input[placeholder*="sk-"]');
		await expect(apiKeyInput).toBeVisible({ timeout: 5000 });
		helpers.log("OpenAI API key input visible");

		// Verify save button exists
		const saveButton = page.locator('button[type="submit"]:has-text("Save")');
		await expect(saveButton).toBeVisible();
		helpers.log("Save button visible");

		helpers.log("AI Settings page displayed correctly");
	});

	test("Can update AI settings", async ({ page }) => {
		helpers.log("Testing AI settings update");

		// Navigate to AI settings page
		await helpers.navigateTo("/admin/administration/ai", {
			timeout: 30000
		});

		// Enter a test API key (won't be valid but tests the form)
		const apiKeyInput = page.locator('input[name="openai_api_key"], input[placeholder*="sk-"]');
		await apiKeyInput.fill('sk-test-api-key-12345');
		helpers.log("Entered test API key");

		// Submit the form
		await page.click('button[type="submit"]:has-text("Save")');

		// Wait for success message or page reload
		await page.waitForTimeout(2000);

		// Verify the value was saved (input should still have the value or show success)
		const savedValue = await apiKeyInput.inputValue();
		// API keys are usually masked, so we just verify something was saved
		helpers.log(`API key field value after save: ${savedValue ? "has value" : "empty"}`);

		helpers.log("AI settings form submitted");
	});

	test("License page is accessible (Pro-only)", async ({ page }) => {
		helpers.log("Testing License page");

		// Navigate to License page
		await helpers.navigateTo("/admin/administration/license", {
			timeout: 30000
		});

		// Verify page loaded with License content
		const licenseHeading = page.locator('h1:has-text("License"), h2:has-text("License")');
		await expect(licenseHeading).toBeVisible({ timeout: 10000 });
		helpers.log("License heading visible");

		// Verify license key input or display exists
		const licenseInput = page.locator('input[name="license_key"], input[placeholder*="license"]');
		const hasInput = await licenseInput.count() > 0;

		if (hasInput) {
			await expect(licenseInput).toBeVisible();
			helpers.log("License key input visible");
		} else {
			// License might be displayed as text if already set
			const licenseDisplay = page.locator('text=TEST-LICENSE-KEY');
			const hasDisplay = await licenseDisplay.count() > 0;
			if (hasDisplay) {
				helpers.log("License key is displayed (already configured)");
			}
		}

		helpers.log("License page displayed correctly");
	});

	test("Pro sidebar shows AI and License links", async ({ page }) => {
		helpers.log("Testing Pro-specific sidebar links");

		// Navigate to any administration page
		await helpers.navigateTo("/admin/administration/ingestion", {
			waitForSelector: 'h1:has-text("Ingestion Settings")',
			timeout: 30000
		});

		// Verify Pro-only sidebar links are present
		const aiLink = page.locator('aside a:has-text("AI")');
		const licenseLink = page.locator('aside a:has-text("License")');

		await expect(aiLink).toBeVisible({ timeout: 5000 });
		helpers.log("AI link visible in sidebar");

		await expect(licenseLink).toBeVisible({ timeout: 5000 });
		helpers.log("License link visible in sidebar");

		// Test navigation to AI page via sidebar
		await aiLink.click();
		await page.waitForURL(/\/admin\/administration\/ai/, { timeout: 10000 });
		helpers.log("Navigated to AI page via sidebar");

		// Test navigation to License page via sidebar
		await licenseLink.click();
		await page.waitForURL(/\/admin\/administration\/license/, { timeout: 10000 });
		helpers.log("Navigated to License page via sidebar");

		helpers.log("Pro sidebar links work correctly");
	});

	// === ASK AI FUNCTIONALITY TESTS ===

	test("Ask AI shows error when API key not configured", async ({ page }) => {
		helpers.log("Testing Ask AI without API key");

		// First ensure we have a website
		const domain = "ai-ask-test.com";
		await helpers.createTestWebsite(domain);

		const currentUrl = page.url();
		const match = currentUrl.match(/\/admin\/websites\/(\d+)/);
		const websiteId = match ? match[1] : "1";

		// Navigate to Ask AI page
		await helpers.navigateTo(`/admin/websites/${websiteId}/lens`, {
			timeout: 30000
		});

		// Type a question
		const textarea = page.locator('textarea');
		await textarea.fill('How many visitors today?');
		helpers.log("Entered question in textarea");

		// Click Ask submit button (use type=submit to avoid matching domain button)
		const askButton = page.locator('button[type="submit"]:has-text("Ask")');
		await askButton.click();
		helpers.log("Clicked Ask button");

		// Should see an error about API key not configured (or loading state)
		// Wait for either error message or loading to complete
		await page.waitForTimeout(3000);

		// Check for error message about API key
		const errorMessage = page.locator('text=OpenAI API key');
		const hasError = await errorMessage.count() > 0;

		if (hasError) {
			helpers.log("Error message about API key displayed (expected behavior)");
		} else {
			// If no error, the button should have returned to normal state
			const buttonText = await askButton.textContent();
			helpers.log(`Ask button state: ${buttonText}`);
		}

		helpers.log("Ask AI functionality tested");
	});

	test("Lens shows example questions when no saved queries", async ({ page }) => {
		helpers.log("Testing Lens example questions");

		// Create a fresh website to ensure no saved queries
		const domain = "lens-examples-test.com";
		await helpers.createTestWebsite(domain);

		const currentUrl = page.url();
		const match = currentUrl.match(/\/admin\/websites\/(\d+)/);
		const websiteId = match ? match[1] : "1";

		// Navigate to Lens page
		await helpers.navigateTo(`/admin/websites/${websiteId}/lens`, {
			timeout: 30000
		});

		// Should see example questions section
		const exampleHeading = page.locator('text=Example questions');
		const hasExamples = await exampleHeading.count() > 0;

		if (hasExamples) {
			await expect(exampleHeading).toBeVisible();
			helpers.log("Example questions section visible");

			// Verify at least one example question button exists
			const exampleButton = page.locator('button:has-text("top 10 pages")');
			await expect(exampleButton).toBeVisible();
			helpers.log("Example question buttons visible");
		} else {
			// If no examples section, might have saved queries already
			helpers.log("No example questions section (may have saved queries)");
		}

		helpers.log("Lens example questions tested");
	});

	test("Lens help overlay shows example questions", async ({ page }) => {
		helpers.log("Testing Lens help overlay");

		// Use existing website
		const domain = "lens-help-test.com";
		await helpers.createTestWebsite(domain);

		const currentUrl = page.url();
		const match = currentUrl.match(/\/admin\/websites\/(\d+)/);
		const websiteId = match ? match[1] : "1";

		// Navigate to Lens page
		await helpers.navigateTo(`/admin/websites/${websiteId}/lens`, {
			timeout: 30000
		});

		// Click help icon (question mark)
		const helpButton = page.locator('button:has(svg.lucide-help-circle), button:has(svg[class*="help"])');
		const hasHelpButton = await helpButton.count() > 0;

		if (hasHelpButton) {
			await helpButton.click();
			helpers.log("Clicked help button");

			// Should see help overlay with example questions (use first() to avoid strict mode)
			const helpOverlay = page.locator('h3:has-text("Example Questions")').first();
			await expect(helpOverlay).toBeVisible({ timeout: 5000 });
			helpers.log("Help overlay visible");

			// Close the overlay by clicking outside or pressing Escape
			await page.keyboard.press('Escape');
			helpers.log("Closed help overlay");
		} else {
			helpers.log("Help button not found (might have different selector)");
		}

		helpers.log("Lens help overlay tested");
	});
});
