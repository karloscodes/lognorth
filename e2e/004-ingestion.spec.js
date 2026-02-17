// e2e/004-ingestion.spec.js - Event ingestion tests
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

test.describe("Event Ingestion Tests", () => {
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

	test("Can ingest events via API", async ({ page }) => {
		helpers.log("Testing event ingestion");

		await helpers.navigateTo("/apps");

		// Get API key from first app
		const apiKeyBtn = page.locator('button:has-text("API Key")').first();
		if (await apiKeyBtn.count() === 0) {
			helpers.log("No apps found, skipping ingestion test", "warn");
			return;
		}

		await apiKeyBtn.click();
		await page.waitForTimeout(500);

		const keyElement = page.locator("code").first();
		const apiKey = await keyElement.textContent();
		helpers.log("Got API key");

		// Send a success event
		const successResponse = await page.evaluate(async (key) => {
			const res = await fetch("/api/v1/events", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${key}`,
				},
				body: JSON.stringify({
					message: "GET /api/health",
					context: { status: 200, path: "/api/health" },
				}),
			});
			return { status: res.status, body: await res.json() };
		}, apiKey);

		expect(successResponse.status).toBe(201);
		expect(successResponse.body.created).toBe(1);
		helpers.log("Success event ingested");

		// Send an error event
		const errorResponse = await page.evaluate(async (key) => {
			const res = await fetch("/api/v1/events", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${key}`,
				},
				body: JSON.stringify({
					message: "TypeError: Cannot read property 'id' of undefined",
					context: { error: "TypeError", path: "/api/users" },
				}),
			});
			return { status: res.status, body: await res.json() };
		}, apiKey);

		expect(errorResponse.status).toBe(201);
		expect(errorResponse.body.created).toBe(1);
		helpers.log("Error event ingested");

		// Navigate to logs page
		const appLink = page.locator('a[href*="/apps/"]').first();
		await appLink.click();
		await page.click('nav >> text=Logs');
		await page.waitForURL(/\/apps\/\d+\/log/);

		// Poll for events to appear (background job runs every 5s)
		helpers.log("Waiting for events to appear in logs...");
		let eventsFound = false;
		for (let i = 0; i < 6; i++) {
			await page.waitForTimeout(2000);
			await page.reload();
			const logContent = await page.textContent("body");
			if (logContent.includes("/api/health") || logContent.includes("TypeError")) {
				eventsFound = true;
				helpers.log(`Events found after ${(i + 1) * 2}s`);
				break;
			}
			helpers.log(`Attempt ${i + 1}: events not yet visible`);
		}

		expect(eventsFound).toBeTruthy();
		helpers.log("Events visible in logs");
	});

	test("Error events create issues", async ({ page }) => {
		helpers.log("Testing error events create issues");

		await helpers.navigateTo("/apps");

		// Get API key
		const apiKeyBtn = page.locator('button:has-text("API Key")').first();
		if (await apiKeyBtn.count() === 0) {
			helpers.log("No apps found, skipping test", "warn");
			return;
		}

		await apiKeyBtn.click();
		await page.waitForTimeout(500);

		const keyElement = page.locator("code").first();
		const apiKey = await keyElement.textContent();

		// Send a unique error event
		const errorMessage = `TestError: Unique error ${Date.now()}`;
		await page.evaluate(async ({ key, msg }) => {
			await fetch("/api/v1/events", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${key}`,
				},
				body: JSON.stringify({
					message: msg,
					context: { error: "TestError", path: "/api/test" },
				}),
			});
		}, { key: apiKey, msg: errorMessage });

		helpers.log("Error event sent");

		// Go to issues page
		const appLink = page.locator('a[href*="/apps/"]').first();
		await appLink.click();
		await page.click('nav >> text=Issues');
		await page.waitForURL(/\/apps\/\d+\/issues/);

		// Poll for issue to appear (background job runs every 5s)
		helpers.log("Waiting for issue to appear...");
		let issueFound = false;
		for (let i = 0; i < 6; i++) {
			await page.waitForTimeout(2000);
			await page.reload();
			const issuesContent = await page.textContent("body");
			if (issuesContent.includes("TestError")) {
				issueFound = true;
				helpers.log(`Issue found after ${(i + 1) * 2}s`);
				break;
			}
			helpers.log(`Attempt ${i + 1}: issue not yet visible`);
		}

		expect(issueFound).toBeTruthy();
		helpers.log("Error event created issue");
	});
});
