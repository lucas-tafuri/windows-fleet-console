import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import os from 'node:os';

test('both installer menu choices reach PowerShell with the right pairing option', { skip: process.platform !== 'win32' }, () => {
  const dir = mkdtempSync(path.join(os.tmpdir(), 'fleet-menu-test-'));
  try {
    const source = readFileSync(new URL('../dist/install.cmd', import.meta.url), 'utf8');
    // Run the actual CMD menu and actual choice.exe. Substitute only the
    // installer invocation so this test cannot enroll or modify this PC.
    const menu = source.slice(source.indexOf('\n:elevated'))
      .replace(/^powershell\.exe.*$/m, 'echo RECEIVED: %PAIRING_OPTION% %*\nexit /b 0');
    const file = path.join(dir, 'menu.cmd');
    writeFileSync(file, '@echo off\r\nsetlocal\r\nset "LAUNCH_LOG=' + path.join(dir, 'launcher.log') + '"\r\n' + menu.replace(/\r?\n/g, '\r\n'));
    for (const [choice, expected] of [['1', 'RECEIVED:'], ['2', 'RECEIVED: -ResetPairing']]) {
      const result = spawnSync('cmd.exe', ['/d', '/c', file], { input: choice + '\r\n', encoding: 'utf8', timeout: 5000 });
      assert.ifError(result.error);
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.ok(result.stdout.includes(expected), result.stdout);
      if (choice === '1') assert.ok(!result.stdout.includes('-ResetPairing'), result.stdout);
    }
    const explicit = spawnSync('cmd.exe', ['/d', '/c', file, '-ResetPairing'], { encoding: 'utf8', timeout: 5000 });
    assert.equal(explicit.status, 0, explicit.stdout + explicit.stderr);
    assert.ok(explicit.stdout.includes('RECEIVED:  -ResetPairing'), explicit.stdout);
  } finally {
    assert.equal(path.dirname(dir), path.resolve(os.tmpdir()));
    rmSync(dir, { recursive: true, force: true });
  }
});
