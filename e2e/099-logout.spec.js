// e2e/099-logout.spec.js - Logout test (runs last)
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

test.describe("Logout Tests", () => {
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

	test("Can logout successfully", async ({ page }) => {
		helpers.log("Testing logout");

		// Find and click logout
		const logoutButton = page.locator('button:has-text("Logout"), a:has-text("Logout"), form[action="/logout"] button');
		await logoutButton.click();

		await page.waitForURL("/login");
		await expect(page.locator('text=Sign in')).toBeVisible();

		helpers.log("Logout successful");
	});

	test("Redirects to login when not authenticated", async ({ page }) => {
		helpers.log("Testing auth redirect");

		// Clear cookies to simulate logged out state
		await page.context().clearCookies();

		await page.goto("/");
		await page.waitForURL("/login");

		await expect(page.locator('text=Sign in')).toBeVisible();
		helpers.log("Correctly redirected to login");
	});

	test("Shows error for invalid credentials", async ({ page }) => {
		helpers.log("Testing invalid login");

		// Clear cookies
		await page.context().clearCookies();

		await helpers.navigateTo("/login");

		await helpers.fillForm(
			{
				email: "wrong@email.com",
				password: "wrongpassword"
			},
			{ waitAfterSubmit: true }
		);

		// Should still be on login page with error
		expect(page.url()).toContain("/login");
		await expect(page.locator('text=Invalid')).toBeVisible();

		helpers.log("Invalid credentials error shown");
	});
});
