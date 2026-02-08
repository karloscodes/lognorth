const { execSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

// Ensure we're using the test environment
process.env.FUSIONALY_ENV = "test";

console.log("=== E2E Test Environment Setup (Pro) ===");
console.log(`Environment: ${process.env.FUSIONALY_ENV}`);
console.log(`Node version: ${process.version}`);
console.log(`Platform: ${process.platform}`);

// Helper function to run commands with better error handling
function runCommand(command, description, options = {}) {
	console.log(`\n📋 ${description}...`);
	try {
		const result = execSync(command, {
			stdio: "inherit",
			encoding: "utf8",
			...options,
		});
		console.log(`✅ ${description} completed successfully`);
		return result;
	} catch (error) {
		console.error(`❌ ${description} failed:`);
		console.error(`Command: ${command}`);
		console.error(`Exit code: ${error.status}`);
		console.error(`Error: ${error.message}`);
		throw error;
	}
}

// Helper function to check if a file exists
function checkFileExists(filePath, description) {
	if (fs.existsSync(filePath)) {
		console.log(`✅ ${description} exists: ${filePath}`);
		return true;
	} else {
		console.log(`❌ ${description} missing: ${filePath}`);
		return false;
	}
}

async function setupTestEnvironment() {
	try {
		// Get the project root directories
		const proRoot = path.resolve(__dirname, "..");
		// OSS is included as git submodule at ./oss
		const ossRoot = path.resolve(proRoot, "oss");

		console.log(`📁 Pro root: ${proRoot}`);
		console.log(`📁 OSS root: ${ossRoot}`);

		// Validate project structure
		const requiredPaths = [
			path.join(proRoot, "go.mod"),
			path.join(proRoot, "cmd/fusionaly-pro/main.go"),
			path.join(ossRoot, "cmd/fnctl/main.go"),
			path.join(ossRoot, "Makefile"),
		];

		for (const requiredPath of requiredPaths) {
			if (!checkFileExists(requiredPath, `Required file ${path.basename(requiredPath)}`)) {
				throw new Error(`Missing required file: ${requiredPath}`);
			}
		}

		// Ensure storage directory exists in Pro (Pro has its own database)
		const storageDir = path.join(proRoot, "storage");
		if (!fs.existsSync(storageDir)) {
			console.log("📁 Creating storage directory...");
			fs.mkdirSync(storageDir, { recursive: true });
		}

		// Build database tools (from Pro Makefile)
		runCommand(
			"make db-build-tools",
			"Building database tools",
			{
				env: { ...process.env, FUSIONALY_ENV: "test" },
				cwd: proRoot,
				timeout: 60000,
			}
		);

		// Verify fnctl was built
		const fnctlPath = path.join(ossRoot, "tmp/fnctl");
		if (!checkFileExists(fnctlPath, "fnctl binary")) {
			throw new Error("fnctl binary was not built successfully");
		}

		// Build Pro web assets (skip npm install if node_modules exists)
		const webNodeModules = path.join(proRoot, "web", "node_modules");
		if (!fs.existsSync(webNodeModules)) {
			runCommand(
				"cd web && npm install --legacy-peer-deps",
				"Installing Pro web dependencies",
				{
					env: { ...process.env, FUSIONALY_ENV: "test" },
					cwd: proRoot,
					timeout: 120000,
				}
			);
		}
		runCommand(
			"cd web && npm run build",
			"Building Pro web assets",
			{
				env: { ...process.env, FUSIONALY_ENV: "test" },
				cwd: proRoot,
				timeout: 120000,
			}
		);

		// Clean the test database directly (OSS Makefile hardcodes storage/ path)
		const testDbPath = path.join(storageDir, "fusionaly-test.db");
		const testDbFiles = [testDbPath, `${testDbPath}-shm`, `${testDbPath}-wal`];
		console.log("\n📋 Dropping test database...");
		for (const file of testDbFiles) {
			if (fs.existsSync(file)) {
				fs.unlinkSync(file);
				console.log(`  Removed: ${path.basename(file)}`);
			}
		}
		console.log("✅ Dropping test database completed successfully");

		// Run migrations for test database (uses Pro Makefile)
		runCommand(
			"make db-migrate",
			"Running database migrations",
			{
				env: { ...process.env, FUSIONALY_ENV: "test" },
				cwd: proRoot,
			}
		);

		// Create localhost website for event ingestion tests
		const dbPath = path.join(storageDir, "fusionaly-test.db");
		runCommand(
			`sqlite3 "${dbPath}" "INSERT OR IGNORE INTO websites (domain, created_at) VALUES ('localhost', datetime('now'));"`,
			"Creating localhost website for event tests",
			{
				env: { ...process.env, FUSIONALY_ENV: "test" },
				cwd: proRoot,
			}
		);

		console.log("\n🎉 E2E test environment setup (Pro) completed successfully!");
		console.log("📊 Setup summary:");
		console.log("  - Database: ✅ Created and migrated");
		console.log("  - Tools: ✅ Built and validated");
		console.log("  - Pro Web: ✅ Built");

	} catch (error) {
		console.error("\n💥 E2E test environment setup failed:");
		console.error(error.message);
		console.error("\n🔍 Troubleshooting tips:");
		console.error("  1. Ensure Go is installed and in PATH");
		console.error("  2. Run 'make install' in fusionaly-oss to install dependencies");
		console.error("  3. Check if port 3000 is available");
		console.error("  4. Verify FUSIONALY_ENV=test is set");
		process.exit(1);
	}
}

// Run setup if this file is executed directly
if (require.main === module) {
	setupTestEnvironment();
}

module.exports = setupTestEnvironment;
