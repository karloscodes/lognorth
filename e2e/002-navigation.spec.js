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

		// First create an app if none exists
		await helpers.navigateTo("/apps");
		const nameInput = page.locator('input[name="name"]');
		if (await nameInput.isVisible({ timeout: 2000 }).catch(() => false)) {
			helpers.log("No apps yet, creating first app");
			await nameInput.fill("Navigation Test App");
			await page.click('button[type="submit"]');
			await page.waitForLoadState("networkidle");
			helpers.log("App created");
		}

		// Now test home page
		await helpers.navigateTo("/");

		// With apps: shows issue count or "All clear"
		const issueCount = page.locator("text=/\\d+ issues?/");
		const allClear = page.locator("text=All clear");
		const hasIssues = await issueCount.isVisible({ timeout: 5000 }).catch(() => false);
		const hasAllClear = await allClear.isVisible({ timeout: 5000 }).catch(() => false);
		expect(hasIssues || hasAllClear).toBeTruthy();
		helpers.log(`Home page: ${hasIssues ? "has issues" : "all clear"}`);
	});

	test("The : command line completes and runs commands", async ({ page }) => {
		helpers.log("Testing the command line");

		// An app to act on
		await helpers.navigateTo("/apps");
		const nameInput = page.locator('input[name="name"]');
		if (await nameInput.isVisible({ timeout: 2000 }).catch(() => false)) {
			await nameInput.fill("Command Line App");
			await page.click('button[type="submit"]');
			await page.waitForLoadState("networkidle");
		}
		await helpers.navigateTo("/");
		const line = page.getByRole("textbox", { name: "Command" });

		// Tab completes the command, then the app name
		await page.keyboard.press(":");
		await expect(line).toBeFocused();
		await page.keyboard.type("ap");
		await page.keyboard.press("Tab");
		await expect(line).toHaveValue("app");
		await page.keyboard.type(" ");
		await page.keyboard.press("Tab");
		await expect(line).toHaveValue(/^app \S+/);
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/apps\/\d+\/issues$/);

		// :since opens the logs from that time, :search keeps it
		await page.keyboard.press(":");
		await page.keyboard.type("since 2h");
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/log\?start_time=/);
		await page.keyboard.press(":");
		await page.keyboard.type("search 503");
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/start_time=.*search=503/);

		// An unknown command says so, like vim
		await page.keyboard.press(":");
		await page.keyboard.type("wq");
		await page.keyboard.press("Enter");
		await expect(page.getByRole("status")).toHaveText("E492: Not an editor command: wq");

		// :q goes home
		await page.keyboard.press(":");
		await page.keyboard.type("q");
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/$/);
	});

	test("Settings is split into sub-pages with sidebar nav", async ({ page }) => {
		helpers.log("Testing settings sub-pages");

		// /settings redirects to /settings/license
		await helpers.navigateTo("/settings");
		await expect(page).toHaveURL(/\/settings\/license$/);

		// Sidebar with the four sub-pages
		const nav = page.locator("nav").filter({ hasText: "License" });
		await expect(nav.getByRole("link", { name: "License" })).toBeVisible();
		await expect(nav.getByRole("link", { name: "Alerts" })).toBeVisible();
		await expect(nav.getByRole("link", { name: "Data" })).toBeVisible();
		await expect(nav.getByRole("link", { name: "Developer" })).toBeVisible();

		// Each sub-page renders its own h2
		await expect(page.locator("h2", { hasText: "License" })).toBeVisible();

		await page.getByRole("link", { name: "Alerts" }).click();
		await expect(page).toHaveURL(/\/settings\/alerts$/);
		await expect(page.locator("h2", { hasText: "Alerts" })).toBeVisible();

		await page.getByRole("link", { name: "Data" }).click();
		await expect(page).toHaveURL(/\/settings\/data$/);
		await expect(page.locator("h2", { hasText: "Data retention" })).toBeVisible();

		await page.getByRole("link", { name: "Developer" }).click();
		await expect(page).toHaveURL(/\/settings\/developer$/);
		await expect(page.locator("h2", { hasText: "AI agents" })).toBeVisible();

		helpers.log("All settings sub-pages reachable via sidebar");
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
