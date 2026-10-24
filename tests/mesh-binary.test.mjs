import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

const binary = fileURLToPath(new URL("../dist/fleet-node.exe", import.meta.url));

test("release binary starts without a Windows console", () => {
  const pe = readFileSync(binary);
  assert.equal(pe.readUInt16LE(0), 0x5a4d, "DOS header");
  const offset = pe.readUInt32LE(0x3c);
  assert.equal(pe.readUInt32LE(offset), 0x4550, "PE signature");
  assert.equal(pe.readUInt16LE(offset + 24), 0x20b, "PE32+ optional header");
  assert.equal(pe.readUInt16LE(offset + 24 + 68), 2, "GUI subsystem: tray must not allocate a console");
});

test("windowless release retains installer CLI output", { skip: process.platform !== "win32" }, () => {
  const result = spawnSync(binary, ["--mesh", "version"], { encoding: "utf8", timeout: 15000 });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout.trim(), /^2\.[\w.+-]+$/);
});
