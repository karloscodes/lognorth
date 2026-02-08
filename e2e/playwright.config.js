// e2e/playwright.config.js
const { devices } = require("@playwright/test");

// Make sure we're using the test environment
process.env.FUSIONALY_ENV = "test";

module.exports = {
	testDir: "./",
	testMatch: [
		"**/*.spec.js",
		"../oss/e2e/**/*.spec.js", // Run OSS E2E tests against Pro server
	],
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	workers: 1,
	reporter: process.env.CI ? "github" : "line",
	timeout: 45000,
	expect: {
		timeout: 8000,
	},
	use: {
		baseURL: "http://localhost:3000",
		trace: process.env.CI ? "on-first-retry" : "off",
		video: "off",
		screenshot: "only-on-failure",
		actionTimeout: 10000,
		navigationTimeout: 20000,
		extraHTTPHeaders: {
			"X-Test-Source": "playwright-e2e-pro",
		},
	},
	projects: [
		{
			name: "chromium",
			use: {
				...devices["Desktop Chrome"],
				viewport: { width: 1280, height: 720 },
				reducedMotion: "reduce",
			},
		},
	],
	webServer: {
		// Run from Pro root - storage defaults to ./storage
		// IMPORTANT: Always start fresh test server to use test database
		command:
			"cd .. && mkdir -p tmp/go-cache && GOCACHE=$(pwd)/tmp/go-cache FUSIONALY_ENV=test FUSIONALY_PUBLIC_DIR=web/dist/assets LOG_LEVEL=error go run cmd/fusionaly-pro/main.go",
		url: "http://localhost:3000/_health",
		reuseExistingServer: false,
		timeout: 90000,
		ignoreHTTPSErrors: true,
		retries: 2,
	},
	globalSetup: require.resolve("./setup-test-env.js"),
};
