// e2e/test-helpers.js
const { expect } = require("@playwright/test");

// Test credentials - matches seed data (admin@localhost / password)
// When running against existing dev server with seed data
const TEST_EMAIL = process.env.TEST_EMAIL || "admin@localhost";
const TEST_PASSWORD = process.env.TEST_PASSWORD || "password";

class TestHelpers {
	constructor(page) {
		this.page = page;
	}

	log(message, level = "info") {
		const timestamp = new Date().toISOString();
		const prefix = level === "error" ? "❌" : level === "warn" ? "⚠️" : "ℹ️";
		console.log(`[${timestamp}] ${prefix} ${message}`);
	}

	async waitForElement(selector, options = {}) {
		const { timeout = 10000, state = "visible", silent = false } = options;
		if (!silent) {
			this.log(`Waiting for element: ${selector}`);
		}

		try {
			await this.page.waitForSelector(selector, { timeout, state });
			if (!silent) {
				this.log(`Element found: ${selector}`);
			}
			return true;
		} catch (error) {
			if (!silent) {
				this.log(`Element not found: ${selector} - ${error.message}`, "error");
			}
			throw error;
		}
	}

	async navigateTo(path, options = {}) {
		const { waitForSelector = null, timeout = 30000 } = options;
		this.log(`Navigating to: ${path}`);

		try {
			await this.page.goto(path, { timeout });
			await this.page.waitForLoadState("networkidle");
			await this.page.waitForTimeout(500);

			if (waitForSelector) {
				await this.waitForElement(waitForSelector);
			}

			this.log(`Navigation successful: ${this.page.url()}`);
		} catch (error) {
			this.log(`Navigation failed: ${path} - ${error.message}`, "error");
			throw error;
		}
	}

	async fillForm(formFields, options = {}) {
		const { submitButton = 'button[type="submit"]', waitAfterSubmit = true } = options;
		this.log(`Filling form with ${Object.keys(formFields).length} fields`);

		try {
			for (const [fieldName, value] of Object.entries(formFields)) {
				const selector = `input[name="${fieldName}"], input[id="${fieldName}"], textarea[name="${fieldName}"], textarea[id="${fieldName}"]`;
				await this.waitForElement(selector);
				await this.page.fill(selector, value);
				this.log(`Filled field: ${fieldName}`);
			}

			if (submitButton) {
				await this.page.click(submitButton);
				this.log("Form submitted");

				if (waitAfterSubmit) {
					await this.page.waitForLoadState("networkidle");
				}
			}
		} catch (error) {
			this.log(`Form filling failed: ${error.message}`, "error");
			throw error;
		}
	}

	async login(email = TEST_EMAIL, password = TEST_PASSWORD, options = {}) {
		const { expectSuccess = true, timeout = 30000 } = options;

		this.log(`Attempting login: ${email}`);

		try {
			await this.navigateTo("/login");

			// Check if redirected to setup (no users exist)
			if (this.page.url().includes("/setup")) {
				throw new Error("Redirected to /setup - onboarding test must run first");
			}

			await this.fillForm({ email, password }, { waitAfterSubmit: false });

			if (expectSuccess) {
				try {
					await this.page.waitForURL(url => !url.href.includes("/login"), { timeout });
				} catch (urlError) {
					await this.page.waitForLoadState("networkidle", { timeout: 5000 });
				}
			} else {
				await this.page.waitForLoadState("networkidle", { timeout });
			}

			const currentUrl = this.page.url();
			this.log(`Post-login URL: ${currentUrl}`);

			if (expectSuccess && currentUrl.includes("/login")) {
				// Check for error messages
				const errorText = await this.page.textContent("body");
				if (errorText.includes("Invalid")) {
					throw new Error("Login failed - invalid credentials");
				}
				throw new Error("Login failed - still on login page");
			}

			this.log("Login successful");
			return { success: true, url: currentUrl };
		} catch (error) {
			this.log(`Login failed: ${error.message}`, "error");
			throw error;
		}
	}

	async cleanup() {
		this.log("Performing test cleanup");
	}
}

module.exports = { TestHelpers, TEST_EMAIL, TEST_PASSWORD };
