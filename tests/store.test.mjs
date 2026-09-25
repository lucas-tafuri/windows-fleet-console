import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import ts from 'typescript';

test('enrollment survives reloads; corrupt state is preserved; failed writes can recover', async () => {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'fleet-store-test-'));
  const previousDir = process.env.FLEET_DATA_DIR;
  const previousToken = process.env.FLEET_TOKEN;
  process.env.FLEET_DATA_DIR = dir;
  process.env.FLEET_TOKEN = 'original-token';
  try {
    for (const name of ['store', 'demo', 'catalog']) {
      const source = await readFile(new URL(`../lib/${name}.ts`, import.meta.url), 'utf8');
      const output = ts.transpileModule(source, {
        compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
      }).outputText.replaceAll('require("./demo")', 'require("./demo.cjs")')
        .replaceAll('require("./catalog")', 'require("./catalog.cjs")');
      await writeFile(path.join(dir, `${name}.cjs`), output);
    }
    const require = createRequire(import.meta.url);
    const storePath = path.join(dir, 'store.cjs');
    const reload = () => { delete require.cache[storePath]; return require(storePath); };
    let api = reload();
    const stores = await Promise.all(Array.from({ length: 10 }, () => api.getStore()));
    assert.ok(stores.every(store => store === stores[0]));
    await api.updateStore(store => { store.joins.push({ id: 'approved-pc', status: 'approved' }); });
    process.env.FLEET_TOKEN = 'different-env-token';
    api = reload();
    assert.equal((await api.getStore()).fleetToken, 'original-token');
    assert.equal((await api.getStore()).joins[0].id, 'approved-pc');
    await assert.rejects(api.updateStore(() => { throw new Error('temporary failure'); }));
    await api.updateStore(store => { store.joins[0].hostname = 'still-paired'; });
    assert.equal((await reload().getStore()).joins[0].hostname, 'still-paired');
    await writeFile(path.join(dir, 'fleet.json'), 'broken');
    await assert.rejects(reload().getStore());
    assert.equal(await readFile(path.join(dir, 'fleet.json'), 'utf8'), 'broken');
  } finally {
    if (previousDir === undefined) delete process.env.FLEET_DATA_DIR;
    else process.env.FLEET_DATA_DIR = previousDir;
    if (previousToken === undefined) delete process.env.FLEET_TOKEN;
    else process.env.FLEET_TOKEN = previousToken;
    assert.equal(path.dirname(dir), path.resolve(os.tmpdir()));
    await rm(dir, { recursive: true, force: true });
  }
});
