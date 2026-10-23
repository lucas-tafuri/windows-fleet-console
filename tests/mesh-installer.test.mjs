import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
test("mesh installer parses and scopes its firewall rules without executing installation", { skip: process.platform !== "win32" }, () => {
  const script = String.raw`
$ErrorActionPreference='Stop'
$tokens=$null; $errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) 'dist/install-mesh.ps1'),[ref]$tokens,[ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$rules=$ast.FindAll({param($n) $n -is [Management.Automation.Language.CommandAst] -and $n.GetCommandName() -eq 'New-NetFirewallRule'},$true)
if ($rules.Count -ne 1) { throw 'Expected one firewall-rule loop' }
if ($rules[0].Extent.Text -notmatch '-Profile Private,Domain' -or $rules[0].Extent.Text -notmatch '-RemoteAddress LocalSubnet') { throw 'Firewall scope changed' }
$choices=$ast.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'Mode' }
if ($choices.Extent.Text -notmatch "ValidateSet\('Create','Join','Update'\)") { throw 'Explicit fleet initialization modes required' }
Write-Output 'Installer verified without executing it'
`;
  const result = spawnSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], { cwd: root, encoding: "utf8", timeout: 15_000 });
  assert.equal(result.status, 0, result.stderr || result.stdout);
});
