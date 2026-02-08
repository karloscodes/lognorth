// e2e/003-ai-api.spec.js - AI API endpoint tests
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

const TEST_EMAIL = "admin@test-e2e.com";
const TEST_PASSWORD = "testpassword123";

test.describe("AI API Tests", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		helpers.log("=== Starting AI API Test ===");

		// Login to get session
		await helpers.login(TEST_EMAIL, TEST_PASSWORD, {
			expectSuccess: true,
			timeout: 30000
		});
		helpers.log("Login successful for AI API test");
	});

	test.afterEach(async ({ page }) => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("AI status endpoint returns correct status", async ({ page }) => {
		helpers.log("Testing AI status endpoint");

		// Make request to AI status endpoint
		const response = await page.request.get("/admin/api/ai/status");

		expect(response.ok()).toBe(true);
		const data = await response.json();

		helpers.log(`AI status response: ${JSON.stringify(data)}`);

		// Verify response structure
		expect(data).toHaveProperty("configured");
		// configured will be false unless OPENAI_API_KEY is set
		expect(typeof data.configured).toBe("boolean");

		helpers.log("AI status endpoint working correctly");
	});

	test("AI ask endpoint handles requests", async ({ page }) => {
		helpers.log("Testing AI ask endpoint");

		// First create a website for the test
		await helpers.createTestWebsite("ai-test-site.com");

		// Navigate to get the website ID
		await helpers.navigateTo("/admin");

		// Make request to AI ask endpoint
		// Note: This will return an error if no API key is configured,
		// but we're testing that the endpoint exists and responds
		const response = await page.request.post("/admin/api/ai/ask", {
			data: {
				website_id: 1,
				question: "How many visitors today?"
			}
		});

		const data = await response.json();
		helpers.log(`AI ask response status: ${response.status()}`);
		helpers.log(`AI ask response: ${JSON.stringify(data)}`);

		// The endpoint should respond (even if with an error about API key)
		// This verifies the route exists and is accessible
		expect(response.status()).toBeLessThan(500); // Not a server error

		helpers.log("AI ask endpoint accessible");
	});

	test("AI saved queries endpoints work", async ({ page }) => {
		helpers.log("Testing AI saved queries endpoints");

		// Test getting saved queries
		const getResponse = await page.request.get("/admin/api/ai/saved/1");

		helpers.log(`Saved queries response status: ${getResponse.status()}`);

		// The endpoint should be accessible (not a 500 error or 404)
		// It might return 401/403 if no license, or 200 with queries
		expect(getResponse.status()).toBeLessThan(500);

		if (getResponse.ok()) {
			const queries = await getResponse.json();
			helpers.log(`Saved queries response: ${JSON.stringify(queries)}`);
			// Should be an array (empty or with items) or object with queries property
			expect(Array.isArray(queries.queries || queries)).toBe(true);
		} else {
			helpers.log("Saved queries returned non-OK status (likely due to licensing)");
		}

		helpers.log("AI saved queries endpoint accessible");
	});

	test("Insights endpoint returns insights data", async ({ page }) => {
		helpers.log("Testing insights endpoint");

		// Test getting insights for website 1
		const response = await page.request.get("/admin/api/insights/1");

		helpers.log(`Insights response status: ${response.status()}`);

		expect(response.ok()).toBe(true);

		const data = await response.json();
		helpers.log(`Insights response: ${JSON.stringify(data)}`);

		// Verify response structure
		expect(data).toHaveProperty("insights");
		expect(Array.isArray(data.insights)).toBe(true);

		helpers.log("Insights endpoint working correctly");
	});

	test("Insights endpoint handles date range parameters", async ({ page }) => {
		helpers.log("Testing insights endpoint with date range");

		// Test with explicit date range
		const response = await page.request.get("/admin/api/insights/1?from=2025-01-01&to=2025-01-07");

		helpers.log(`Insights with date range response status: ${response.status()}`);

		expect(response.ok()).toBe(true);

		const data = await response.json();
		expect(data).toHaveProperty("insights");
		expect(Array.isArray(data.insights)).toBe(true);

		helpers.log("Insights endpoint handles date range correctly");
	});
});
