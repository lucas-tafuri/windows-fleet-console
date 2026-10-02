import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { inspectFleet, inspectDirectory, restoreFleet } from '../scripts/repair-fleet-store.mjs';

test('zero-filled state is preserved and restored only from valid fleet data', async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'fleet-recovery-test-'));
  try {
    const damaged = Buffer.alloc(512);
    const original = JSON.stringify({ fleetToken: 'saved-secret', machines: { pc: { hostname: 'PC' } }, jobs: [] });
    await writeFile(path.join(dir, 'fleet.json'), damaged);
    await writeFile(path.join(dir, 'fleet.json.tmp'), 'broken');
    await writeFile(path.join(dir, 'fleet.json.bak'), original);
    const findings = await inspectDirectory(dir);
    assert.equal(findings.find(f => f.file === 'fleet.json').zeroBytes, 512);
    assert.equal(JSON.stringify(findings).includes('saved-secret'), false);
    assert.equal(inspectFleet(Buffer.from('{}')).valid, false);
    await assert.rejects(restoreFleet(dir, path.join(dir, 'fleet.json.tmp')));
    assert.deepEqual(await readFile(path.join(dir, 'fleet.json')), damaged);
    const result = await restoreFleet(dir, path.join(dir, 'fleet.json.bak'));
    assert.deepEqual(await readFile(result.preserved), damaged);
    assert.equal(await readFile(result.restored, 'utf8'), original);
    assert.equal(await readFile(path.join(dir, 'fleet.json.bak'), 'utf8'), original);
    await assert.rejects(restoreFleet(dir, path.join(dir, 'fleet.json.bak')), /Current fleet.json is valid/);
  } finally {
    assert.equal(path.dirname(dir), path.resolve(os.tmpdir()));
    await rm(dir, { recursive: true, force: true });
  }
});
