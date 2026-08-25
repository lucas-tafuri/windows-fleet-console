import type { Job, JobKind, Machine, StoreData } from "./types";

export const DEMO_MACHINES: Machine[] = [
  {
    id: "demo-studio",
    hostname: "STUDIO-01",
    user: "maya",
    os: "Windows 11 Pro",
    demo: true,
    lastSeen: Date.now(),
    cpu: 11,
    memory: 42,
    frozenHint: false,
    metricsLimited: false,
    lastInputAgeMs: 4_000,
    mappedDrives: [{ letter: "S", path: "\\\\nas\\projects" }],
    transport: "demo",
  },
  {
    id: "demo-lab",
    hostname: "LAB-04",
    user: "jules",
    os: "Windows 11 Pro",
    demo: true,
    lastSeen: Date.now(),
    cpu: 91,
    memory: 76,
    frozenHint: false,
    metricsLimited: false,
    lastInputAgeMs: 1_200,
    mappedDrives: [],
    transport: "demo",
  },
  {
    id: "demo-front",
    hostname: "FRONT-DESK",
    user: "reception",
    os: "Windows 10 Pro",
    demo: true,
    lastSeen: Date.now(),
    cpu: 6,
    memory: 58,
    frozenHint: true,
    metricsLimited: false,
    lastInputAgeMs: 180_000,
    mappedDrives: [{ letter: "Z", path: "\\\\files\\shared" }],
    transport: "demo",
  },
];

export function seedDemoMachines(store: StoreData): void {
  for (const machine of DEMO_MACHINES) {
    if (!store.machines[machine.id]) {
      store.machines[machine.id] = { ...machine, lastSeen: Date.now() };
    }
  }
}

export function hasRealAgents(store: StoreData): boolean {
  return Object.values(store.machines).some((m) => !m.demo);
}

export function tickDemoMachines(store: StoreData): void {
  if (hasRealAgents(store)) return;
  const now = Date.now();
  for (const machine of Object.values(store.machines)) {
    if (!machine.demo) continue;
    machine.lastSeen = now;
    machine.transport = "demo";
    if (machine.id === "demo-studio") {
      machine.cpu = 8 + Math.round(Math.random() * 10);
      machine.memory = 40 + Math.round(Math.random() * 6);
      machine.frozenHint = false;
    } else if (machine.id === "demo-lab") {
      machine.cpu = 86 + Math.round(Math.random() * 10);
      machine.memory = 72 + Math.round(Math.random() * 8);
      machine.frozenHint = false;
    } else if (machine.id === "demo-front") {
      machine.cpu = 4 + Math.round(Math.random() * 5);
      machine.memory = 55 + Math.round(Math.random() * 6);
      machine.frozenHint = true;
    }
  }
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function installedStory(hostname: string, pkg: string) {
  const needle = pkg.toLowerCase();
  if (hostname === "LAB-04" && (needle.includes("vlc") || needle.includes("videolan"))) {
    return true;
  }
  if (hostname === "STUDIO-01" && (needle.includes("chrome") || needle.includes("notepad"))) {
    return true;
  }
  return false;
}

export async function simulateDemoResult(job: Job, machine: Machine) {
  await sleep(700 + Math.random() * 1400);
  const pkg = job.payload.package || "";
  const letter = (job.payload.letter || "Z").toUpperCase();
  const kind: JobKind = job.kind;

  if (kind === "check") {
    const found = installedStory(machine.hostname, pkg);
    return {
      status: "ok" as const,
      via: machine.hostname === "FRONT-DESK" ? "registry" : "winget",
      message: found
        ? `${pkg} is installed on ${machine.hostname}`
        : `${pkg} is not installed on ${machine.hostname}`,
      output: found
        ? `Found ${pkg} in the package inventory.`
        : `No matching package id or display name.`,
    };
  }

  if (kind === "install") {
    return {
      status: "ok" as const,
      via: "winget",
      message: `Installed ${pkg} on ${machine.hostname}`,
      output: `winget install --id ${pkg} --silent\nSuccessfully installed`,
    };
  }

  if (kind === "uninstall") {
    const found = installedStory(machine.hostname, pkg);
    if (!found) {
      return {
        status: "ok" as const,
        via: "winget",
        message: `${pkg} was not installed on ${machine.hostname}`,
        output: "No installed package matched.",
      };
    }
    return {
      status: "ok" as const,
      via: "winget",
      message: `Uninstalled ${pkg} from ${machine.hostname}`,
      output: `winget uninstall --id ${pkg}\nSuccessfully uninstalled`,
    };
  }

  if (kind === "map_drive") {
    return {
      status: "ok" as const,
      via: "net use",
      message: `Mapped ${letter}: to ${job.payload.unc || "\\\\server\\share"}`,
      output: `The command completed successfully.`,
    };
  }

  if (kind === "unmap_drive") {
    return {
      status: "ok" as const,
      via: "net use",
      message: `Unmapped ${letter}:`,
      output: `${letter}: was deleted successfully.`,
    };
  }

  if (kind === "clean_downloads") {
    const deleted = 12 + Math.floor(Math.random() * 30);
    const skipped = Math.floor(Math.random() * 3);
    return {
      status: "ok" as const,
      via: "filesystem",
      message: `Deleted ${deleted} files, skipped ${skipped} locked`,
      output: `${machine.user}\\Downloads cleaned.`,
    };
  }

  if (kind === "empty_recycle") {
    return {
      status: "ok" as const,
      via: "SHEmptyRecycleBin",
      message: `Recycle Bin emptied on ${machine.hostname}`,
      output: "SHEmptyRecycleBin succeeded.",
    };
  }

  if (kind === "self_update") {
    return {
      status: "ok" as const,
      via: machine.hostname === "FRONT-DESK" ? "github download" : "git pull",
      message: `Updated and restarting agent on ${machine.hostname}`,
      output: `git pull origin main\nAlready registered at Windows logon.`,
    };
  }

  return {
    status: "ok" as const,
    via: "ShellExecute",
    message: `Launched ${job.payload.target || "app"} on ${machine.hostname}`,
    output: `Started ${job.payload.target}`,
  };
}
