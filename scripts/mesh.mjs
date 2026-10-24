import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const go = process.env.GO || "go";
const task = process.argv[2];
const commands = {
  contracts: ["run", "./cmd/contracts", "../lib/mesh-contracts.ts"],
  test: ["test", "./...", "-count=1", "-timeout", "180s"],
  // The same Windows binary hosts the service and the interactive tray.
  build: ["build", "-trimpath", "-ldflags=-s -w -H windowsgui", "-o", "../dist/fleet-node.exe", "."],
};
if (!commands[task]) throw new Error("Use contracts, test, or build");
const result = spawnSync(go, commands[task], { cwd: path.join(root, "agent"), stdio: "inherit", env: { ...process.env, CGO_ENABLED: "0", ...(task === "build" ? { GOOS: "windows", GOARCH: "amd64" } : {}) } });
if (result.error) console.error(`Go toolchain required (set GO to its full path): ${result.error.message}`);
process.exit(result.status ?? 1);
