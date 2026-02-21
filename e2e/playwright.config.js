// e2e/playwright.config.js
const { devices } = require("@playwright/test");

// Make sure we're using the test environment
process.env.LOGNORTH_ENV = "test";

module.exports = {
	testDir: "./",
	testMatch: ["**/*.spec.js"], // All tests run in numeric order (001, 002, etc.)
	fullyParallel: false, // Sequential execution due to test dependencies
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	workers: 1, // Single worker - tests have dependencies (onboarding must run first)
	reporter: process.env.CI ? "github" : "line",
	timeout: 45000,
	expect: {
		timeout: 8000,
	},
	use: {
		baseURL: "http://localhost:8080",
		trace: process.env.CI ? "on-first-retry" : "off",
		video: "off",
		screenshot: "only-on-failure",
		actionTimeout: 10000,
		navigationTimeout: 20000,
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
		command: process.env.CI
			? "cd .. && LOGNORTH_ENV=test ./tmp/lognorth-server"
			: "cd .. && LOGNORTH_ENV=test go run cmd/lognorth/main.go",
		url: "http://localhost:8080/login",
		reuseExistingServer: !process.env.CI,
		timeout: 90000,
		env: {
			...process.env,
			LOGNORTH_ENV: "test",
		},
	},
	globalSetup: require.resolve("./setup-test-env.js"),
};
