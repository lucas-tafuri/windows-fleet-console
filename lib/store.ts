import { mkdir, open, readFile, rename, rm } from "fs/promises";
import path from "path";
import { randomBytes } from "crypto";
import type { MapPrefs, StoreData } from "./types";
import { seedDemoMachines } from "./demo";
import { DEFAULT_SOFTWARE } from "./catalog";

const DATA_DIR = process.env.FLEET_DATA_DIR || path.join(process.cwd(), "data");
const FILE = path.join(DATA_DIR, "fleet.json");
const BACKUP = path.join(DATA_DIR, "fleet.json.bak");

let cache: StoreData | null = null;
let loading: Promise<StoreData> | null = null;
let chain: Promise<void> = Promise.resolve();

function emptyMapPrefs(): MapPrefs {
  return { letter: "Z", unc: "", username: "", password: "", uncHistory: [] };
}

function normalizeUnc(path: string) {
  return path.replace(/\//g, "\\").replace(/\\+$/, "").toLowerCase();
}

function backfillMapPrefs(store: StoreData): boolean {
  const prefs = store.mapPrefs;
  if (!prefs) return false;
  if (prefs.unc || (prefs.uncHistory && prefs.uncHistory.length > 0)) return false;
  let changed = false;
  for (const job of [...(store.jobs || [])].reverse()) {
    if (job.kind !== "map_drive" || !job.payload.unc) continue;
    const unc = job.payload.unc.trim();
    if (!unc) continue;
    const want = normalizeUnc(unc);
    if (!prefs.uncHistory.some((u) => normalizeUnc(u) === want)) {
      prefs.uncHistory.push(unc);
      changed = true;
    }
    if (!prefs.unc) {
      prefs.unc = unc;
      changed = true;
    }
    if (job.payload.letter && prefs.letter === "Z") {
      prefs.letter = job.payload.letter.replace(/:$/, "").toUpperCase();
      changed = true;
    }
    if (job.payload.username && !prefs.username) {
      prefs.username = job.payload.username;
      changed = true;
    }
    if (job.payload.password && !prefs.password) {
      prefs.password = job.payload.password;
      changed = true;
    }
  }
  prefs.uncHistory = prefs.uncHistory.slice(0, 12);
  return changed;
}

function emptyStore(): StoreData {
  return {
    fleetToken: process.env.FLEET_TOKEN || randomBytes(24).toString("base64url"),
    machines: {},
    jobs: [],
    software: DEFAULT_SOFTWARE.map((item) => ({ ...item })),
    softwareStatus: {},
    joins: [],
    mapPrefs: emptyMapPrefs(),
  };
}

async function loadFromDisk(): Promise<StoreData> {
  let raw: string;
  try {
    raw = await readFile(FILE, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    // A missing primary beside a backup is a recovery case, not a new fleet.
    try {
      await readFile(BACKUP);
    } catch (backupError) {
      if ((backupError as NodeJS.ErrnoException).code !== "ENOENT") throw backupError;
      const store = emptyStore();
      seedDemoMachines(store);
      await persist(store);
      return store;
    }
    throw new Error("Fleet data is missing but a backup exists. Run scripts/repair-fleet-store.mjs to inspect recovery options.");
  }
  try {
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
    if (!parsed.mapPrefs) {
      parsed.mapPrefs = emptyMapPrefs();
      dirty = true;
    }
    if (!parsed.mapPrefs.uncHistory) {
      parsed.mapPrefs.uncHistory = [];
      dirty = true;
    }
    if (backfillMapPrefs(parsed)) dirty = true;
    if (!parsed.fleetToken) {
      parsed.fleetToken =
        process.env.FLEET_TOKEN || randomBytes(24).toString("base64url");
      dirty = true;
    }
    if (dirty) await persist(parsed);
    return parsed;
  } catch (error) {
    // Do not include parser excerpts, which can contain saved credentials.
    if (error instanceof SyntaxError) {
      throw new Error("Saved fleet data is invalid; it has been left unchanged. Run scripts/repair-fleet-store.mjs to inspect recovery options.");
    }
    throw error;
  }
}

async function durableReplace(file: string, json: string): Promise<void> {
  const temporary = `${file}.${process.pid}-${randomBytes(8).toString("hex")}.tmp`;
  try {
    const handle = await open(temporary, "wx", 0o600);
    try {
      await handle.writeFile(json, "utf8");
      await handle.sync();
    } finally {
      await handle.close();
    }
    await rename(temporary, file);
  } finally {
    await rm(temporary, { force: true });
  }
}

async function persist(store: StoreData): Promise<void> {
  await mkdir(DATA_DIR, { recursive: true });
  const json = JSON.stringify(store, null, 2);
  let previous: string | undefined;
  try {
    previous = await readFile(FILE, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
  // Save the previous complete state before replacing the primary. If the
  // primary was damaged externally, refuse the write and retain the backup.
  if (previous !== undefined) JSON.parse(previous);
  await durableReplace(BACKUP, previous ?? json);
  await durableReplace(FILE, json);
}

export async function getStore(): Promise<StoreData> {
  if (!cache) {
    loading ??= loadFromDisk().catch((error) => {
      loading = null;
      throw error;
    });
    cache = await loading;
  }
  if (Object.keys(cache.machines).length === 0) {
    seedDemoMachines(cache);
  }
  return cache;
}

export async function updateStore(
  mutator: (store: StoreData) => void | boolean
): Promise<StoreData> {
  let result: StoreData | null = null;
  chain = chain.catch(() => {}).then(async () => {
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
