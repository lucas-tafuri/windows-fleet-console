import { randomBytes } from "crypto";
import { getStore, updateStore } from "./store";
import { deriveStatus, lastSeenLabel } from "./status";
import {
  hasRealAgents,
  seedDemoMachines,
  simulateDemoResult,
  tickDemoMachines,
} from "./demo";
import { agentTokenOk, isUnlocked, pinRequired } from "./auth";
import type {
  AgentJobResult,
  AssignedJob,
  FleetSnapshot,
  Heartbeat,
  Job,
  JobKind,
  JobPayload,
  Machine,
  MachineView,
} from "./types";

function newId(prefix: string) {
  return `${prefix}_${randomBytes(8).toString("hex")}`;
}

export async function getSnapshot(opts?: {
  unlocked?: boolean;
}): Promise<FleetSnapshot> {
  const store = await getStore();
  tickDemoMachines(store);
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
    demoActive,
    pinRequired: pinRequired(),
    unlocked,
    unprotected: !pinRequired(),
    serverTime: now,
  };
}

export async function getEnrollInfo(hostHeader: string | null) {
  const store = await getStore();
  const proto =
    process.env.FLEET_PUBLIC_URL?.replace(/\/$/, "") ||
    guessPublicUrl(hostHeader);
  return {
    token: store.fleetToken,
    serverUrl: proto,
    command: `fleet-agent.exe --server ${proto} --token ${store.fleetToken}`,
    pollFallback: `fleet-agent.exe --server ${proto} --token ${store.fleetToken} --http-only`,
    scheduledTask:
      "First run copies the agent to %LOCALAPPDATA%\\FleetConsole and registers a logon scheduled task (Startup folder fallback).",
    repo: "https://github.com/lucas-tafuri/windows-fleet-console.git",
  };
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
  jobs: Job[],
  machine: Machine,
  results: AgentJobResult[] | undefined
) {
  if (!results?.length) return;
  for (const incoming of results) {
    const job = jobs.find((j) => j.id === incoming.jobId);
    if (!job) continue;
    const slot = job.results[machine.id];
    if (!slot) continue;
    slot.status = incoming.status;
    slot.via = incoming.via;
    slot.message = incoming.message;
    slot.output = incoming.output;
    slot.finishedAt = Date.now();
    const values = Object.values(job.results);
    if (values.every((r) => r.status === "ok" || r.status === "error")) {
      job.status = "done";
    }
  }
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
    applyAgentResults(s.jobs, machine, hb.results);
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
    job.results[id] = {
      machineId: id,
      hostname: machine.hostname,
      status: "queued",
      message: "Waiting for agent",
    };
  }

  await updateStore((s) => {
    s.jobs.push(job);
    if (s.jobs.length > 120) s.jobs.splice(0, s.jobs.length - 120);
  });

  for (const id of ids) {
    const machine = store.machines[id];
    if (!machine?.demo) continue;
    void runDemo(job.id, id);
  }

  return job;
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
    if (j.kind === "map_drive" && m && j.payload.letter && j.payload.unc) {
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
