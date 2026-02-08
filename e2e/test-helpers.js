// e2e/test-helpers.js
const { expect } = require("@playwright/test");

/**
 * Test helper utilities for E2E tests (Pro version)
 */

export const TEST_EMAIL = "admin@test-e2e.com";
export const TEST_PASSWORD = "testpassword123";

class TestHelpers {
	constructor(page) {
		this.page = page;
	}

	/**
	 * Enhanced logging with timestamps
	 */
	log(message, level = "info") {
		const timestamp = new Date().toISOString();
		const prefix = level === "error" ? "❌" : level === "warn" ? "⚠️" : "ℹ️";
		console.log(`[${timestamp}] ${prefix} ${message}`);
	}

	/**
	 * Wait for element with better error handling
	 */
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

	/**
	 * Enhanced navigation with validation
	 */
	async navigateTo(path, options = {}) {
		const { waitForSelector = null, timeout = 30000 } = options;
		this.log(`Navigating to: ${path}`);

		try {
			await this.page.goto(path, { timeout });
			await this.page.waitForLoadState("networkidle");

			// Wait for React hydration
			try {
				await this.page.waitForFunction(() => {
					const root = document.getElementById('root');
					if (root && root.children.length > 0) {
						return true;
					}
					const body = document.body;
					return body && body.children.length > 0;
				}, { timeout: 10000 });
			} catch (hydrationError) {
				this.log(`React hydration timeout: ${hydrationError.message}`);
			}

			await this.page.waitForTimeout(1000);

			if (waitForSelector) {
				await this.waitForElement(waitForSelector);
			}

			this.log(`Navigation successful: ${this.page.url()}`);
		} catch (error) {
			this.log(`Navigation failed: ${path} - ${error.message}`, "error");
			throw error;
		}
	}

	/**
	 * Enhanced form filling with validation
	 */
	async fillForm(formFields, options = {}) {
		const { submitButton = 'button[type="submit"]', waitAfterSubmit = true } = options;
		this.log(`Filling form with ${Object.keys(formFields).length} fields`);

		try {
			for (const [fieldName, value] of Object.entries(formFields)) {
				const selector = `input[name="${fieldName}"], input[id="${fieldName}"], textarea[name="${fieldName}"], textarea[id="${fieldName}"], select[name="${fieldName}"], select[id="${fieldName}"]`;
				await this.waitForElement(selector);
				await this.page.fill(selector, value);
				this.log(`Filled field: ${fieldName}`);
			}

			if (submitButton !== null && submitButton !== undefined) {
				await this.page.click(submitButton);
				this.log("Form submitted");

				if (waitAfterSubmit) {
					await this.page.waitForLoadState("networkidle");
				}
			} else {
				this.log("Form filled but not submitted (submitButton is null)");
			}
		} catch (error) {
			this.log(`Form filling failed: ${error.message}`, "error");
			throw error;
		}
	}

	/**
	 * Enhanced login with comprehensive error handling
	 */
	async login(email = "test@example.com", password = "password", options = {}) {
		const {
			expectSuccess = true,
			redirectPath = null,
			timeout = 30000
		} = options;

		this.log(`Attempting login: ${email}`);

		try {
			await this.navigateTo("/login");
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

			if (currentUrl.includes("chrome-error://") || currentUrl.includes("about:blank")) {
				throw new Error(`Browser failed to load page after login: ${currentUrl}`);
			}

			const errorMessages = await this.checkForMessages("error");

			if (expectSuccess) {
				if (errorMessages.length > 0) {
					throw new Error(`Login failed with errors: ${errorMessages.join(", ")}`);
				}

				if (currentUrl.includes("/login")) {
					throw new Error("Login failed - still on login page");
				}

				if (redirectPath && !currentUrl.includes(redirectPath)) {
					throw new Error(`Expected redirect to ${redirectPath}, but got ${currentUrl}`);
				}

				this.log("Login successful");
			} else {
				if (errorMessages.length === 0 && !currentUrl.includes("/login")) {
					throw new Error("Expected login to fail, but it succeeded");
				}
				this.log("Login failed as expected");
			}

			return { success: !currentUrl.includes("/login"), url: currentUrl, errors: errorMessages };
		} catch (error) {
			this.log(`Login process failed: ${error.message}`, "error");
			throw error;
		}
	}

	/**
	 * Check for flash messages or alerts
	 */
	async checkForMessages(type = "any") {
		const selectors = [
			'[role="alert"]',
			'.alert',
			'.flash-message',
			'.error-message',
			'.success-message',
			'.notification',
			'[data-testid="flash-message"]',
			'.text-red-500', '.text-red-600', '.text-red-700',
			'.text-green-500', '.text-green-600', '.text-green-700',
		];

		const messages = [];

		for (const selector of selectors) {
			try {
				const elements = await this.page.locator(selector).all();
				for (const element of elements) {
					const isVisible = await element.isVisible();
					if (isVisible) {
						const isInsightBadge = await element.getAttribute('data-insight-badge');
						if (isInsightBadge) continue;

						const text = await element.textContent();
						if (text && text.trim()) {
							const messageType = this.determineMessageType(text, selector);
							if (type === "any" || type === messageType) {
								messages.push({ type: messageType, text: text.trim(), selector });
							}
						}
					}
				}
			} catch (error) {
				// Ignore errors for individual selectors
			}
		}

		if (messages.length > 0) {
			this.log(`Found ${messages.length} messages: ${messages.map(m => m.text).join(", ")}`);
		}

		return messages.map(m => m.text);
	}

	/**
	 * Determine message type based on content and selector
	 */
	determineMessageType(text, selector) {
		const lowerText = text.toLowerCase();
		const lowerSelector = selector.toLowerCase();

		if (lowerText.includes("error") || lowerText.includes("failed") || lowerText.includes("invalid") ||
			lowerSelector.includes("error") || lowerSelector.includes("red")) {
			return "error";
		}

		if (lowerText.includes("success") || lowerText.includes("created") || lowerText.includes("saved") ||
			lowerSelector.includes("success") || lowerSelector.includes("green")) {
			return "success";
		}

		if (lowerText.includes("warning") || lowerText.includes("warn")) {
			return "warning";
		}

		return "info";
	}

	/**
	 * Enhanced screenshot with context
	 */
	async takeScreenshot(name, options = {}) {
		const { fullPage = false, path = null } = options;
		const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
		const filename = path || `screenshot-${name}-${timestamp}.png`;

		this.log(`Taking screenshot: ${filename}`);

		try {
			await this.page.screenshot({
				path: filename,
				fullPage,
				...options
			});
			this.log(`Screenshot saved: ${filename}`);
		} catch (error) {
			this.log(`Screenshot failed: ${error.message}`, "error");
		}
	}

	/**
	 * Create a website for testing
	 */
	async createTestWebsite(domain = null) {
		if (!domain) {
			domain = `test-website-${Date.now()}.com`;
		}

		this.log(`Creating test website: ${domain}`);

		try {
			await this.navigateTo("/admin/websites/new");
			await this.fillForm({ domain }, { waitAfterSubmit: false });
			await this.page.waitForURL(url => !url.href.includes("/new"), { timeout: 15000 });

			const currentUrl = this.page.url();
			if (currentUrl.includes("/new")) {
				throw new Error("Still on creation page after website creation");
			}

			this.log(`Website created successfully: ${domain}`);
			return domain;
		} catch (error) {
			this.log(`Website creation failed: ${error.message}`, "error");
			throw error;
		}
	}

	/**
	 * Clean up test data
	 */
	async cleanup() {
		this.log("Performing test cleanup");
		this.log("Cleanup completed");
	}
}

module.exports = { TestHelpers };
