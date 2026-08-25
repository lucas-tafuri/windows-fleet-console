import { mkdir, readFile, rename, writeFile } from "fs/promises";
import path from "path";
import { randomBytes } from "crypto";
import type { StoreData } from "./types";
import { seedDemoMachines } from "./demo";
import { DEFAULT_SOFTWARE } from "./catalog";

const DATA_DIR = path.join(process.cwd(), "data");
const FILE = path.join(DATA_DIR, "fleet.json");
const TMP = path.join(DATA_DIR, "fleet.json.tmp");

let cache: StoreData | null = null;
let chain: Promise<void> = Promise.resolve();

function emptyStore(): StoreData {
  return {
    fleetToken: process.env.FLEET_TOKEN || randomBytes(24).toString("base64url"),
    machines: {},
    jobs: [],
    software: DEFAULT_SOFTWARE.map((item) => ({ ...item })),
    softwareStatus: {},
    joins: [],
  };
}

async function loadFromDisk(): Promise<StoreData> {
  try {
    const raw = await readFile(FILE, "utf8");
    const parsed = JSON.parse(raw) as StoreData;
    if (!parsed.machines) parsed.machines = {};
    if (!parsed.jobs) parsed.jobs = [];
    let dirty = false;
    if (!parsed.software) {
      parsed.software = DEFAULT_SOFTWARE.map((item) => ({ ...item }));
      dirty = true;
    }
    if (!parsed.softwareStatus) {
      parsed.softwareStatus = {};
      dirty = true;
    }
    if (!parsed.joins) {
      parsed.joins = [];
      dirty = true;
    }
    if (!parsed.fleetToken) {
      parsed.fleetToken =
        process.env.FLEET_TOKEN || randomBytes(24).toString("base64url");
      dirty = true;
    }
    if (process.env.FLEET_TOKEN) parsed.fleetToken = process.env.FLEET_TOKEN;
    if (dirty) await persist(parsed);
    return parsed;
  } catch {
    const store = emptyStore();
    seedDemoMachines(store);
    await persist(store);
    return store;
  }
}

async function persist(store: StoreData): Promise<void> {
  await mkdir(DATA_DIR, { recursive: true });
  const json = JSON.stringify(store, null, 2);
  await writeFile(TMP, json, "utf8");
  await rename(TMP, FILE);
}

export async function getStore(): Promise<StoreData> {
  if (!cache) cache = await loadFromDisk();
  if (Object.keys(cache.machines).length === 0) {
    seedDemoMachines(cache);
  }
  return cache;
}

export async function updateStore(
  mutator: (store: StoreData) => void | boolean
): Promise<StoreData> {
  let result: StoreData | null = null;
  chain = chain.then(async () => {
    const store = await getStore();
    const persistNeeded = mutator(store);
    if (persistNeeded !== false) await persist(store);
    result = store;
  });
  await chain;
  return result ?? (await getStore());
}

export function peekStore(): StoreData | null {
  return cache;
}
