// Read-only unless --restore explicitly names a validated recovery file.
import { copyFile, mkdir, open, readFile, readdir, rename, rm } from 'node:fs/promises';
import { constants } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';

export function inspectFleet(raw) {
  let value;
  try { value = JSON.parse(raw.toString('utf8')); }
  catch { return { valid: false, reason: 'Invalid JSON', bytes: raw.length, zeroBytes: raw.reduce((n, b) => n + (b === 0 ? 1 : 0), 0) }; }
  if (!value || typeof value !== 'object' ||
      typeof value.fleetToken !== 'string' || !value.fleetToken ||
      !value.machines || typeof value.machines !== 'object' || Array.isArray(value.machines) ||
      !Array.isArray(value.jobs) ||
      Object.values(value.machines).some(m => !m || typeof m.hostname !== 'string') ||
      value.jobs.some(j => !j || !Array.isArray(j.machineIds) || !j.payload || !j.results)) {
    return { valid: false, reason: 'Missing or invalid fleet fields', bytes: raw.length };
  }
  return { valid: true, bytes: raw.length, machines: Object.keys(value.machines).length, jobs: value.jobs.length };
}

export async function inspectDirectory(dir) {
  const names = (await readdir(dir)).filter(name => name.startsWith('fleet') && !name.endsWith('.mjs')).sort();
  const results = [];
  for (const name of names) {
    try { results.push({ file: name, ...inspectFleet(await readFile(path.join(dir, name))) }); }
    catch { results.push({ file: name, valid: false, reason: 'Cannot read file' }); }
  }
  return results;
}

export async function restoreFleet(dir, candidate) {
  const primary = path.resolve(dir, 'fleet.json');
  if (path.resolve(candidate) === primary) throw new Error('Select a recovery copy, not fleet.json itself.');
  const raw = await readFile(candidate);
  if (!inspectFleet(raw).valid) throw new Error('Selected file is not a valid fleet recovery copy. Nothing changed.');
  await mkdir(dir, { recursive: true });
  let damaged;
  try { damaged = await readFile(primary); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (damaged && inspectFleet(damaged).valid) throw new Error('Current fleet.json is valid; refusing to replace it.');
  const suffix = randomUUID();
  const preserved = damaged === undefined ? null : `${primary}.damaged-${suffix}`;
  const temp = `${primary}.restore-${suffix}.tmp`;
  // Keep an exact copy of the damaged original for further recovery.
  if (preserved) {
    await copyFile(primary, preserved, constants.COPYFILE_EXCL);
    const handle = await open(preserved, 'r+');
    try { await handle.sync(); } finally { await handle.close(); }
  }
  try {
    const handle = await open(temp, 'wx', 0o600);
    try { await handle.writeFile(raw); await handle.sync(); }
    finally { await handle.close(); }
    await rename(temp, primary);
  } finally { await rm(temp, { force: true }); }
  return { restored: primary, preserved };
}

async function main() {
  const args = process.argv.slice(2);
  const dir = path.resolve(args.shift() || process.env.FLEET_DATA_DIR || 'data');
  if (args.length && (args.length !== 2 || args[0] !== '--restore')) {
    throw new Error('Usage: node scripts/repair-fleet-store.mjs [data-directory] [--restore recovery-file]');
  }
  if (args[0] === '--restore') {
    console.log(JSON.stringify(await restoreFleet(dir, path.resolve(dir, args[1])), null, 2));
    console.log('Recovery complete. Restart the host. Changes newer than the recovery copy are not included.');
  } else {
    console.log(JSON.stringify(await inspectDirectory(dir), null, 2));
    console.log('Inspection only; nothing changed. Stop the host before using --restore with a valid recovery file.');
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { console.error(error.message); process.exitCode = 1; });
}
