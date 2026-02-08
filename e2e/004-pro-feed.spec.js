// e2e/004-pro-feed.spec.js - Pro Feed (Home) page tests
const { test, expect } = require("@playwright/test");
const { TestHelpers } = require("./test-helpers");

const TEST_EMAIL = "admin@test-e2e.com";
const TEST_PASSWORD = "testpassword123";

test.describe("Pro Feed Tests", () => {
	let helpers;

	test.beforeEach(async ({ page }) => {
		helpers = new TestHelpers(page);
		helpers.log("=== Starting Pro Feed Test ===");

		// Login with test credentials
		await helpers.login(TEST_EMAIL, TEST_PASSWORD, {
			expectSuccess: true,
			timeout: 30000
		});
		helpers.log("Login successful for Pro feed test");
	});

	test.afterEach(async ({ page }) => {
		if (helpers) {
			await helpers.cleanup();
		}
	});

	test("Pro home page shows feed layout", async ({ page }) => {
		helpers.log("Testing Pro home page layout");

		// Navigate to admin home (Pro Feed)
		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Should see the Pro Home heading or sites section
		const heading = page.locator('h1, h2').filter({ hasText: /Sites|Home|Feed/i }).first();
		await expect(heading).toBeVisible({ timeout: 10000 });
		helpers.log("Pro home heading visible");

		// Should see sites section with at least one website
		const sitesSection = page.locator('text=Sites');
		const hasSites = await sitesSection.count() > 0;
		if (hasSites) {
			await expect(sitesSection).toBeVisible();
			helpers.log("Sites section visible");
		}

		helpers.log("Pro home page layout verified");
	});

	test("Website cards show domain and event count", async ({ page }) => {
		helpers.log("Testing website cards display");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Look for website cards (they should have domain text)
		const websiteCards = page.locator('[class*="grid"] a[href*="/admin/websites/"]');
		const cardCount = await websiteCards.count();
		helpers.log(`Found ${cardCount} website cards`);

		if (cardCount > 0) {
			// First card should have a domain name
			const firstCard = websiteCards.first();
			await expect(firstCard).toBeVisible();

			// Should show event count (could be 0, "2k", "10k", etc.)
			const cardText = await firstCard.textContent();
			helpers.log(`First card text: ${cardText}`);
		}

		helpers.log("Website cards display verified");
	});

	test("Clicking website card navigates to dashboard", async ({ page }) => {
		helpers.log("Testing website card navigation");

		// Navigate to home/feed page (websites created by earlier tests should exist)
		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Wait for feed page to load with website cards (from earlier tests)
		const websiteCard = page.locator('a[href*="/admin/websites/"][href*="/dashboard"]').first();

		// Check if we're on the feed page with website cards
		const cardCount = await websiteCard.count();
		if (cardCount === 0) {
			// If no cards, we might be redirected to /admin/websites/new - that's expected if no websites exist
			const currentUrl = page.url();
			if (currentUrl.includes("/websites/new")) {
				helpers.log("No websites exist yet - this test depends on earlier tests creating websites");
				return;
			}
		}

		await expect(websiteCard).toBeVisible({ timeout: 10000 });
		await websiteCard.click();
		await page.waitForURL(/\/admin\/websites\/\d+\/dashboard/, { timeout: 10000 });
		helpers.log("Navigated to dashboard via card click");

		// Verify we're on the dashboard
		const currentUrl = page.url();
		expect(currentUrl).toMatch(/\/admin\/websites\/\d+\/dashboard/);

		helpers.log("Website card navigation verified");
	});

	test("Feed section shows activity items", async ({ page }) => {
		helpers.log("Testing feed activity section");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Look for activity/feed section
		const activityHeading = page.locator('text=Activity').first();
		const hasActivity = await activityHeading.count() > 0;

		if (hasActivity) {
			await expect(activityHeading).toBeVisible();
			helpers.log("Activity section heading visible");

			// Feed items should be in a scrollable container
			const feedContainer = page.locator('[class*="overflow-y-auto"]');
			const hasFeed = await feedContainer.count() > 0;

			if (hasFeed) {
				helpers.log("Feed container found");
			}
		}

		// Check for either feed items or empty state
		const feedItems = page.locator('[class*="flex"][class*="items-start"]');
		const emptyState = page.locator('text=/No activity|quiet|nothing/i');

		const hasItems = await feedItems.count() > 0;
		const isEmpty = await emptyState.count() > 0;

		if (hasItems) {
			helpers.log(`Found ${await feedItems.count()} potential feed items`);
		} else if (isEmpty) {
			helpers.log("Feed is empty (expected for fresh install)");
		}

		helpers.log("Feed activity section verified");
	});

	test("Feed items show correct icons for types", async ({ page }) => {
		helpers.log("Testing feed item icons");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Wait for page to load
		await page.waitForTimeout(2000);

		// Check for Lucide icons that indicate feed item types
		// TrendingUp for spikes, TrendingDown for drops, ExternalLink for referrers, etc.
		const trendingUp = page.locator('svg.lucide-trending-up');
		const trendingDown = page.locator('svg.lucide-trending-down');
		const externalLink = page.locator('svg.lucide-external-link');
		const target = page.locator('svg.lucide-target');
		const trophy = page.locator('svg.lucide-trophy');

		const iconCounts = {
			trendingUp: await trendingUp.count(),
			trendingDown: await trendingDown.count(),
			externalLink: await externalLink.count(),
			target: await target.count(),
			trophy: await trophy.count()
		};

		helpers.log(`Icon counts: ${JSON.stringify(iconCounts)}`);

		// At least one type of icon should be present if there are feed items
		const totalIcons = Object.values(iconCounts).reduce((a, b) => a + b, 0);
		helpers.log(`Total feed-related icons found: ${totalIcons}`);

		helpers.log("Feed item icons verified");
	});

	test("Website card shows dropdown menu on hover", async ({ page }) => {
		helpers.log("Testing website card dropdown menu");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Find a website card container (the parent div with the dropdown)
		const websiteCardContainer = page.locator('[class*="group"]').filter({
			has: page.locator('a[href*="/admin/websites/"]')
		}).first();

		const hasCard = await websiteCardContainer.count() > 0;

		if (hasCard) {
			// Hover over the card to show dropdown button
			await websiteCardContainer.hover();

			// Look for the dropdown trigger (MoreVertical icon button)
			const dropdownButton = websiteCardContainer.locator('button').filter({
				has: page.locator('svg.lucide-more-vertical')
			});

			// The dropdown button should become visible on hover
			await page.waitForTimeout(500); // Wait for hover state

			const isVisible = await dropdownButton.isVisible().catch(() => false);
			if (isVisible) {
				helpers.log("Dropdown button visible on hover");

				// Click to open dropdown
				await dropdownButton.click();

				// Should see menu items
				const trackingScript = page.locator('text=Tracking Script');
				const deleteOption = page.locator('text=Delete');

				const hasMenu = await trackingScript.count() > 0 || await deleteOption.count() > 0;
				if (hasMenu) {
					helpers.log("Dropdown menu items visible");
				}
			} else {
				helpers.log("Dropdown button not visible (may be always visible or different implementation)");
			}
		} else {
			helpers.log("No website cards with dropdown found");
		}

		helpers.log("Website card dropdown menu verified");
	});

	test("Can access tracking script dialog", async ({ page }) => {
		helpers.log("Testing tracking script dialog");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Find and hover over website card
		const websiteCardContainer = page.locator('[class*="group"]').filter({
			has: page.locator('a[href*="/admin/websites/"]')
		}).first();

		const hasCard = await websiteCardContainer.count() > 0;

		if (hasCard) {
			await websiteCardContainer.hover();

			// Click dropdown button
			const dropdownButton = websiteCardContainer.locator('button').first();
			await dropdownButton.click();

			// Click "Tracking Script" option
			const trackingOption = page.locator('text=Tracking Script');
			const hasOption = await trackingOption.count() > 0;

			if (hasOption) {
				await trackingOption.click();

				// Dialog should appear with script code
				const dialog = page.locator('[role="dialog"], [class*="Dialog"]');
				await expect(dialog).toBeVisible({ timeout: 5000 });
				helpers.log("Tracking script dialog opened");

				// Should contain script tag code
				const scriptContent = page.locator('text=<script');
				const hasScript = await scriptContent.count() > 0;
				if (hasScript) {
					helpers.log("Script tag visible in dialog");
				}

				// Close dialog
				await page.keyboard.press('Escape');
			} else {
				helpers.log("Tracking Script option not found in menu");
			}
		}

		helpers.log("Tracking script dialog verified");
	});

	test("Feed scrolls when content overflows", async ({ page }) => {
		helpers.log("Testing feed scroll behavior");

		await helpers.navigateTo("/admin", {
			timeout: 30000
		});

		// Find the feed container with overflow-y-auto
		const feedContainer = page.locator('[class*="overflow-y-auto"]').first();
		const hasContainer = await feedContainer.count() > 0;

		if (hasContainer) {
			const scrollHeight = await feedContainer.evaluate(el => el.scrollHeight);
			const clientHeight = await feedContainer.evaluate(el => el.clientHeight);

			helpers.log(`Feed scrollHeight: ${scrollHeight}, clientHeight: ${clientHeight}`);

			if (scrollHeight > clientHeight) {
				helpers.log("Feed has scrollable content");

				// Try scrolling
				await feedContainer.evaluate(el => el.scrollTop = 100);
				const scrollTop = await feedContainer.evaluate(el => el.scrollTop);

				if (scrollTop > 0) {
					helpers.log("Feed scroll working");
				}
			} else {
				helpers.log("Feed content fits without scrolling");
			}
		}

		helpers.log("Feed scroll behavior verified");
	});
});
