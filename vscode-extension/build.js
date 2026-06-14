const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const root = __dirname;

console.log("=== Codecast VS Code Extension Build ===\n");

// 1. Check that node_modules exist
if (!fs.existsSync(path.join(root, "node_modules"))) {
  console.log("Installing dependencies...");
  execSync("npm install", { cwd: root, stdio: "inherit" });
}

// 2. Clean output directory
const outDir = path.join(root, "out");
if (fs.existsSync(outDir)) {
  fs.rmSync(outDir, { recursive: true, force: true });
}
fs.mkdirSync(outDir, { recursive: true });

// 3. Compile TypeScript
console.log("\nCompiling TypeScript...");
try {
  execSync("npx tsc -p ./", { cwd: root, stdio: "inherit" });
  console.log("\nBuild succeeded!");
} catch (err) {
  console.error("\nBuild FAILED.");
  process.exit(1);
}
