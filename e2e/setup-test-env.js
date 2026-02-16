// e2e/setup-test-env.js
const { execSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

process.env.LOGNORTH_ENV = "test";

console.log("=== LogNorth E2E Test Environment Setup ===");
console.log(`Environment: ${process.env.LOGNORTH_ENV}`);

function runCommand(command, description, options = {}) {
	console.log(`\n📋 ${description}...`);
	try {
		execSync(command, {
			stdio: "inherit",
			encoding: "utf8",
			...options,
		});
		console.log(`✅ ${description} completed successfully`);
	} catch (error) {
		console.error(`❌ ${description} failed: ${error.message}`);
		throw error;
	}
}

async function setupTestEnvironment() {
	try {
		const projectRoot = path.resolve(__dirname, "..");
		console.log(`📁 Project root: ${projectRoot}`);

		// Ensure storage directory exists
		const storageDir = path.join(projectRoot, "storage");
		if (!fs.existsSync(storageDir)) {
			console.log("📁 Creating storage directory...");
			fs.mkdirSync(storageDir, { recursive: true });
		}

		// Remove test database to start fresh
		const testDbPath = path.join(storageDir, "lognorth-test.db");
		if (fs.existsSync(testDbPath)) {
			console.log("🗑️ Removing existing test database...");
			fs.unlinkSync(testDbPath);
			// Also remove WAL files
			const walPath = testDbPath + "-wal";
			const shmPath = testDbPath + "-shm";
			if (fs.existsSync(walPath)) fs.unlinkSync(walPath);
			if (fs.existsSync(shmPath)) fs.unlinkSync(shmPath);
		}

		// Build web assets
		runCommand(
			"cd web && npm run build",
			"Building web assets",
			{
				cwd: projectRoot,
				timeout: 120000,
			}
		);

		console.log("\n🎉 E2E test environment setup completed!");
		console.log("📊 Summary:");
		console.log("  - Database: Fresh (will be created on server start)");
		console.log("  - Web assets: Built");
		console.log("  - Server: Will start via webServer config");

	} catch (error) {
		console.error("\n💥 E2E test environment setup failed:");
		console.error(error.message);
		process.exit(1);
	}
}

module.exports = setupTestEnvironment;
