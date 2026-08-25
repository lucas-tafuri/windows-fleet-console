import { randomBytes } from "crypto";
import os from "os";
import { getStore, updateStore } from "./store";
import { deriveStatus, lastSeenLabel } from "./status";
import {
  hasRealAgents,
  seedDemoMachines,
  simulateDemoResult,
  tickDemoMachines,
} from "./demo";
import { agentTokenOk, isUnlocked, pinRequired } from "./auth";
import { slugSoftwareId } from "./catalog";
import type {
  AgentJobResult,
  AssignedJob,
  CatalogApp,
  FleetSnapshot,
  Heartbeat,
  Job,
  JobKind,
  JobPayload,
  JoinRequest,
  Machine,
  MachineView,
  StoreData,
} from "./types";

export const JOIN_TTL_MS = 10 * 60 * 1000;

function newId(prefix: string) {
  return `${prefix}_${randomBytes(8).toString("hex")}`;
}

export async function getSnapshot(opts?: {
  unlocked?: boolean;
}): Promise<FleetSnapshot> {
  const store = await getStore();
  tickDemoMachines(store);
  pruneJoins(store);
  const demoActive = !hasRealAgents(store);
  if (demoActive) seedDemoMachines(store);

  const now = Date.now();
  const unlocked = opts?.unlocked ?? (await isUnlocked());
  const machines: MachineView[] = Object.values(store.machines)
    .filter((m) => (demoActive ? m.demo : !m.demo))
    .map((m) => ({
      ...m,
      status: deriveStatus(m, now),
      lastSeenLabel: lastSeenLabel(m.lastSeen, now),
    }))
    .sort((a, b) => a.hostname.localeCompare(b.hostname));

  const jobs = store.jobs
    .filter((job) =>
      demoActive
        ? job.machineIds.some((id) => store.machines[id]?.demo)
        : job.machineIds.some((id) => store.machines[id] && !store.machines[id].demo)
    )
    .slice(-80)
    .reverse();

  return {
    machines,
    jobs,
    software: store.software || [],
    softwareStatus: store.softwareStatus || {},
    pendingJoins: (store.joins || []).filter((j) => j.status === "pending"),
    mapPrefs: store.mapPrefs || {
      letter: "Z",
      unc: "",
      username: "",
      password: "",
      uncHistory: [],
    },
    demoActive,
    pinRequired: pinRequired(),
    unlocked,
    unprotected: !pinRequired(),
    serverTime: now,
  };
}

export async function getEnrollInfo(hostHeader: string | null) {
  const store = await getStore();
  const port = process.env.PORT || "43123";
  const lanUrls = lanHttpUrls(port);
  const fromHeader = guessPublicUrl(hostHeader);
  const localhost =
    fromHeader.includes("127.0.0.1") || fromHeader.includes("localhost");
  const proto =
    process.env.FLEET_PUBLIC_URL?.replace(/\/$/, "") ||
    (localhost && lanUrls[0] ? lanUrls[0] : fromHeader);
  return {
    token: store.fleetToken,
    serverUrl: proto,
    lanUrls,
    localhostHint: localhost && !process.env.FLEET_PUBLIC_URL,
    command: `powershell -NoProfile -ExecutionPolicy Bypass -File install.ps1`,
    pollFallback: `powershell -NoProfile -ExecutionPolicy Bypass -File install.ps1 -HttpOnly`,
    scheduledTask:
      "install.ps1 / install.cmd copies the PrettyDamnFleet agent to %LOCALAPPDATA%\\FleetConsole and registers a logon task (Startup folder fallback).",
    repo: "https://github.com/lucas-tafuri/windows-fleet-console.git",
    oneLiner: `powershell -NoExit -NoProfile -ExecutionPolicy Bypass -Command "Write-Host 'Log will be at' $env:TEMP\\fleet-console-install.log; iwr -UseBasicParsing https://raw.githubusercontent.com/lucas-tafuri/windows-fleet-console/main/dist/install.ps1 -OutFile $env:TEMP\\fleet-install.ps1; & $env:TEMP\\fleet-install.ps1"`,
  };
}

function lanHttpUrls(port: string): string[] {
  const urls: string[] = [];
  for (const list of Object.values(os.networkInterfaces())) {
    for (const a of list || []) {
      const v4 = a.family === "IPv4" || (a.family as unknown) === 4;
      if (!v4 || a.internal || !a.address) continue;
      urls.push(`http://${a.address}:${port}`);
    }
  }
  return urls;
}

function guessPublicUrl(hostHeader: string | null) {
  const host = hostHeader || `127.0.0.1:${process.env.PORT || "43123"}`;
  if (host.includes("localhost") || host.startsWith("127.") || host.startsWith("0.0.0.0")) {
    return `http://${host.replace("0.0.0.0", "127.0.0.1")}`;
  }
  const tls = process.env.FLEET_TLS === "1";
  return `${tls ? "https" : "http"}://${host}`;
}

function applyHeartbeat(machine: Machine | undefined, hb: Heartbeat, transport: "ws" | "poll"): Machine {
  const id = hb.machineId || machine?.id || newId("pc");
  return {
    id,
    hostname: hb.hostname || machine?.hostname || id,
    user: hb.user || machine?.user || "",
    os: hb.os || machine?.os || "Windows",
    demo: false,
    lastSeen: Date.now(),
    cpu: hb.cpu ?? machine?.cpu ?? null,
    memory: hb.memory ?? machine?.memory ?? null,
    frozenHint: Boolean(hb.frozen),
    metricsLimited: Boolean(hb.metricsLimited),
    lastInputAgeMs: hb.lastInputAgeMs ?? null,
    mappedDrives: hb.mappedDrives || machine?.mappedDrives || [],
    transport,
  };
}

function pendingJobsFor(store: { jobs: Job[] }, machineId: string): AssignedJob[] {
  const assigned: AssignedJob[] = [];
  for (const job of store.jobs) {
    const result = job.results[machineId];
    if (!result || result.status !== "queued") continue;
    result.status = "running";
    result.startedAt = Date.now();
    result.message = "Running on agent";
    job.status = "running";
    assigned.push({ id: job.id, kind: job.kind, payload: job.payload });
  }
  return assigned;
}

function applyAgentResults(
  store: { jobs: Job[]; software: CatalogApp[]; softwareStatus: StoreData["softwareStatus"] },
  machine: Machine,
  results: AgentJobResult[] | undefined
) {
  if (!results?.length) return;
  for (const incoming of results) {
    const job = store.jobs.find((j) => j.id === incoming.jobId);
    if (!job) continue;
    const slot = job.results[machine.id];
    if (!slot) continue;
    slot.status = incoming.status;
    slot.via = incoming.via;
    slot.message = incoming.message;
    slot.output = incoming.output;
    slot.finishedAt = Date.now();
    recordSoftwareStatus(store, machine.id, job, incoming);
    const values = Object.values(job.results);
    if (values.every((r) => r.status === "ok" || r.status === "error")) {
      job.status = "done";
    }
  }
}

function matchCatalog(software: CatalogApp[], pkg: string) {
  const n = pkg.trim().toLowerCase();
  if (!n) return undefined;
  return software.find(
    (item) =>
      item.match.toLowerCase() === n ||
      item.name.toLowerCase() === n ||
      (item.wingetId && item.wingetId.toLowerCase() === n)
  );
}

function recordSoftwareStatus(
  store: { software: CatalogApp[]; softwareStatus: StoreData["softwareStatus"] },
  machineId: string,
  job: Job,
  result: AgentJobResult
) {
  if (job.kind !== "check" && job.kind !== "install" && job.kind !== "uninstall") return;
  const pkg = job.payload.package || "";
  const item = matchCatalog(store.software || [], pkg);
  if (!item) return;
  if (!store.softwareStatus) store.softwareStatus = {};
  if (!store.softwareStatus[machineId]) store.softwareStatus[machineId] = {};

  let installed: boolean | null = null;
  if (job.kind === "install") {
    installed = result.status === "ok";
  } else if (job.kind === "uninstall") {
    installed = result.status === "ok" ? false : null;
  } else {
    const msg = result.message.toLowerCase();
    if (msg.includes("not installed")) installed = false;
    else if (msg.includes("is installed")) installed = true;
  }
  if (installed == null) return;

  store.softwareStatus[machineId][item.id] = {
    installed,
    lastChecked: Date.now(),
    via: result.via,
    detail: result.message,
  };
}

export async function handleAgentPoll(
  hb: Heartbeat,
  transport: "ws" | "poll"
): Promise<{ ok: true; machineId: string; jobs: AssignedJob[] } | { ok: false; error: string }> {
  const store = await getStore();
  if (!agentTokenOk(hb.token, store.fleetToken)) {
    return { ok: false, error: "Invalid fleet token" };
  }
  if (!hb.hostname && !hb.machineId) {
    return { ok: false, error: "hostname or machineId required" };
  }

  let machineId = "";
  let assigned: AssignedJob[] = [];

  await updateStore((s) => {
    let existing = hb.machineId ? s.machines[hb.machineId] : undefined;
    if (!existing && hb.hostname) {
      existing = Object.values(s.machines).find(
        (m) => !m.demo && m.hostname === hb.hostname && m.user === (hb.user || "")
      );
    }
    const machine = applyHeartbeat(existing, hb, transport);
    s.machines[machine.id] = machine;
    applyAgentResults(s, machine, hb.results);
    assigned = pendingJobsFor(s, machine.id);
    machineId = machine.id;
  });

  return { ok: true, machineId, jobs: assigned };
}

export async function createJob(input: {
  kind: JobKind;
  payload: JobPayload;
  machineIds: string[];
}): Promise<Job> {
  const store = await getStore();
  const ids = input.machineIds.filter((id) => store.machines[id]);
  if (ids.length === 0) throw new Error("No matching machines");

  const job: Job = {
    id: newId("job"),
    kind: input.kind,
    createdAt: Date.now(),
    payload: { ...input.payload },
    machineIds: ids,
    results: {},
    status: "queued",
  };
  for (const id of ids) {
    const machine = store.machines[id];
    const skip =
      input.kind === "map_drive"
        ? alreadyMappedMessage(machine, input.payload)
        : null;
    job.results[id] = skip
      ? {
          machineId: id,
          hostname: machine.hostname,
          status: "ok",
          via: "already mapped",
          message: skip,
          finishedAt: Date.now(),
        }
      : {
          machineId: id,
          hostname: machine.hostname,
          status: "queued",
          message: "Waiting for agent",
        };
  }
  if (
    Object.values(job.results).every((r) => r.status === "ok" || r.status === "error")
  ) {
    job.status = "done";
  }

  await updateStore((s) => {
    s.jobs.push(job);
    if (s.jobs.length > 120) s.jobs.splice(0, s.jobs.length - 120);
    if (input.kind === "map_drive") rememberMapPrefs(s, input.payload);
  });

  for (const id of ids) {
    const machine = store.machines[id];
    if (!machine?.demo) continue;
    if (job.results[id]?.status !== "queued") continue;
    void runDemo(job.id, id);
  }

  return job;
}

function rememberMapPrefs(store: StoreData, payload: JobPayload) {
  const unc = (payload.unc || "").trim();
  const letter = (payload.letter || "").replace(/:$/, "").toUpperCase();
  if (!store.mapPrefs) {
    store.mapPrefs = {
      letter: "Z",
      unc: "",
      username: "",
      password: "",
      uncHistory: [],
    };
  }
  if (letter) store.mapPrefs.letter = letter;
  if (unc) {
    store.mapPrefs.unc = unc;
    const want = normalizeUnc(unc);
    store.mapPrefs.uncHistory = [
      unc,
      ...(store.mapPrefs.uncHistory || []).filter((u) => normalizeUnc(u) !== want),
    ].slice(0, 12);
  }
  if (payload.username) store.mapPrefs.username = payload.username;
  if (payload.password) store.mapPrefs.password = payload.password;
}

function normalizeUnc(path: string) {
  return path.replace(/\//g, "\\").replace(/\\+$/, "").toLowerCase();
}

function alreadyMappedMessage(machine: Machine, payload: JobPayload): string | null {
  const letter = (payload.letter || "").replace(/:$/, "").toUpperCase();
  const unc = (payload.unc || "").trim();
  if (!letter || !unc) return null;
  const want = normalizeUnc(unc);
  const byLetter = (machine.mappedDrives || []).find(
    (d) => d.letter.toUpperCase() === letter
  );
  if (byLetter) {
    return `Already mapped ${letter}: to ${byLetter.path}; skipped`;
  }
  const byUnc = (machine.mappedDrives || []).find(
    (d) => normalizeUnc(d.path) === want
  );
  if (byUnc) {
    return `Already mapped ${byUnc.letter}: to ${byUnc.path}; skipped`;
  }
  return null;
}

async function runDemo(jobId: string, machineId: string) {
  const store = await getStore();
  const job = store.jobs.find((j) => j.id === jobId);
  const machine = store.machines[machineId];
  if (!job || !machine) return;

  await updateStore((s) => {
    const j = s.jobs.find((x) => x.id === jobId);
    const r = j?.results[machineId];
    if (!j || !r) return false;
    r.status = "running";
    r.startedAt = Date.now();
    r.message = "Simulated agent running";
    j.status = "running";
  });

  const outcome = await simulateDemoResult(job, machine);

  await updateStore((s) => {
    const j = s.jobs.find((x) => x.id === jobId);
    const r = j?.results[machineId];
    const m = s.machines[machineId];
    if (!j || !r) return false;
    r.status = outcome.status;
    r.via = outcome.via;
    r.message = outcome.message;
    r.output = outcome.output;
    r.finishedAt = Date.now();
    recordSoftwareStatus(s, machineId, j, {
      jobId: j.id,
      status: outcome.status,
      via: outcome.via,
      message: outcome.message,
      output: outcome.output,
    });
    if (
      j.kind === "map_drive" &&
      m &&
      j.payload.letter &&
      j.payload.unc &&
      outcome.via !== "already mapped"
    ) {
      const letter = j.payload.letter.toUpperCase();
      m.mappedDrives = [
        ...m.mappedDrives.filter((d) => d.letter !== letter),
        { letter, path: j.payload.unc },
      ];
    }
    if (j.kind === "unmap_drive" && m && j.payload.letter) {
      const letter = j.payload.letter.toUpperCase();
      m.mappedDrives = m.mappedDrives.filter((d) => d.letter !== letter);
    }
    const values = Object.values(j.results);
    if (values.every((x) => x.status === "ok" || x.status === "error")) {
      j.status = "done";
    }
  });
}

export async function clearJobs() {
  await updateStore((s) => {
    s.jobs = [];
  });
}

export async function addCatalogApp(input: {
  name: string;
  match?: string;
  wingetId?: string;
}): Promise<CatalogApp> {
  const name = input.name.trim();
  if (!name) throw new Error("Name is required");
  const match = (input.match || name).trim();
  const wingetId = input.wingetId?.trim() || undefined;
  const idBase = slugSoftwareId(name);
  let app: CatalogApp | null = null;
  await updateStore((s) => {
    if (!s.software) s.software = [];
    let id = idBase;
    let n = 2;
    while (s.software.some((item) => item.id === id)) {
      id = `${idBase}-${n++}`;
    }
    app = { id, name, match, wingetId };
    s.software.push(app);
  });
  if (!app) throw new Error("Could not add software");
  return app;
}

export async function removeCatalogApp(id: string) {
  await updateStore((s) => {
    s.software = (s.software || []).filter((item) => item.id !== id);
    for (const machineId of Object.keys(s.softwareStatus || {})) {
      delete s.softwareStatus[machineId][id];
    }
  });
}

function pruneJoins(store: StoreData, now = Date.now()) {
  if (!store.joins) store.joins = [];
  store.joins = store.joins.filter((j) => {
    const age = now - j.createdAt;
    if (j.status === "pending") return age <= JOIN_TTL_MS;
    const decidedAge = now - (j.decidedAt || j.createdAt);
    return decidedAge <= JOIN_TTL_MS;
  });
}

export async function createJoinRequest(input: {
  hostname: string;
  user: string;
  os: string;
  ip: string;
}): Promise<JoinRequest> {
  const hostname = input.hostname.trim();
  const user = input.user.trim();
  if (!hostname) throw new Error("hostname required");
  const now = Date.now();
  let created: JoinRequest | null = null;
  await updateStore((s) => {
    pruneJoins(s, now);
    const existing = s.joins.find(
      (j) =>
        j.status === "pending" &&
        j.hostname.toLowerCase() === hostname.toLowerCase() &&
        j.user.toLowerCase() === user.toLowerCase()
    );
    if (existing) {
      existing.ip = input.ip || existing.ip;
      existing.os = input.os || existing.os;
      created = existing;
      return;
    }
    created = {
      id: newId("join"),
      hostname,
      user,
      os: input.os.trim() || "Windows",
      ip: input.ip.trim(),
      createdAt: now,
      status: "pending",
    };
    s.joins.push(created);
    if (s.joins.length > 40) s.joins.splice(0, s.joins.length - 40);
  });
  if (!created) throw new Error("Could not create join request");
  return created;
}

export async function getJoin(id: string): Promise<JoinRequest | undefined> {
  const store = await getStore();
  pruneJoins(store);
  return (store.joins || []).find((j) => j.id === id);
}

export async function decideJoin(
  id: string,
  action: "approve" | "deny"
): Promise<JoinRequest> {
  const now = Date.now();
  let found: JoinRequest | undefined;
  await updateStore((s) => {
    pruneJoins(s, now);
    const join = (s.joins || []).find((j) => j.id === id);
    if (!join) return false;
    if (join.status !== "pending") {
      found = join;
      return false;
    }
    join.status = action === "approve" ? "approved" : "denied";
    join.decidedAt = now;
    found = join;
  });
  if (!found) throw new Error("Join request not found or expired");
  return found;
}

export function publicJoinView(
  join: JoinRequest,
  opts?: { token?: string; server?: string }
) {
  if (join.status === "approved") {
    return {
      id: join.id,
      status: join.status,
      hostname: join.hostname,
      server: opts?.server,
      token: opts?.token,
    };
  }
  return {
    id: join.id,
    status: join.status,
    hostname: join.hostname,
  };
}
