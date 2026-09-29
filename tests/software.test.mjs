import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import ts from 'typescript';

test('software versions survive client results and persistence without losing legacy support', async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'fleet-software-test-'));
  const previousDir = process.env.FLEET_DATA_DIR;
  process.env.FLEET_DATA_DIR = dir;
  const require = createRequire(import.meta.url);
  const modules = new Map();
  function load(name) {
    if (modules.has(name)) return modules.get(name).exports;
    const loaded = { exports: {} };
    modules.set(name, loaded);
    const source = readFileSync(new URL(`../lib/${name}.ts`, import.meta.url), 'utf8');
    const { outputText } = ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
    });
    new Function('exports', 'require', 'module', outputText)(loaded.exports,
      name => name.startsWith('./') ? load(name.slice(2)) : require(name), loaded);
    return loaded.exports;
  }
  try {
    const hub = load('hub');
    const store = await load('store').getStore();
    const token = store.fleetToken;
    const client = await hub.handleAgentPoll({ token, hostname: 'VERSION-PC' }, 'poll');
    const second = await hub.handleAgentPoll({ token, hostname: 'OTHER-PC' }, 'ws');
    const app = await hub.addCatalogApp({ name: 'Adobe Photoshop', wingetId: 'Adobe.Photoshop' });
    const check = async (kind, result) => {
      const machineId = client.machineId;
      const job = await hub.createJob({ kind, payload: { package: app.wingetId }, machineIds: [machineId] });
      const poll = await hub.handleAgentPoll({ token, machineId, hostname: 'VERSION-PC' }, 'poll');
      if (kind === 'check') assert.equal(poll.jobs[0].payload.match, 'Adobe Photoshop');
      await hub.handleAgentPoll({ token, machineId, hostname: 'VERSION-PC', results: [{ jobId: job.id, ...result }] }, 'ws');
      return store.softwareStatus[machineId]?.[app.id];
    };
    const installations = [
      { name: 'Adobe Photoshop 2025', version: '26.8.1' },
      { name: 'Adobe Photoshop 2026', version: '27.0.2' },
    ];
    const first = await check('check', { status: 'ok', message: 'Inventory complete', installed: true, installations });
    assert.equal(first.installed, true);
    assert.deepEqual(first.installations, installations);
    assert.equal(store.softwareStatus[second.machineId], undefined);
    assert.deepEqual((await hub.getSnapshot({ unlocked: true })).softwareStatus[client.machineId][app.id].installations, installations);
    const saved = JSON.parse(await readFile(path.join(dir, 'fleet.json'), 'utf8'));
    assert.deepEqual(saved.softwareStatus[client.machineId][app.id].installations, installations);
    for (const kind of ['check', 'install', 'uninstall']) {
      assert.deepEqual(await check(kind, { status: 'error', message: 'not installed: scan failed' }), first);
    }
    const missing = await check('check', { status: 'ok', message: 'Inventory complete', installed: false });
    assert.equal(missing.installed, false);
    assert.equal(missing.installations, undefined);
    const legacy = await check('check', { status: 'ok', message: 'Adobe.Photoshop is installed' });
    assert.equal(legacy.installed, true);
    assert.equal(legacy.installations, undefined);
    const removed = await check('uninstall', { status: 'ok', message: 'Uninstalled' });
    assert.equal(removed.installed, false);
    assert.equal(removed.installations, undefined);
  } finally {
    if (previousDir === undefined) delete process.env.FLEET_DATA_DIR;
    else process.env.FLEET_DATA_DIR = previousDir;
    assert.equal(path.dirname(dir), path.resolve(os.tmpdir()));
    await rm(dir, { recursive: true, force: true });
  }
});
