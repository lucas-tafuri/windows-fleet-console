import type { Machine, MachineStatus } from "./types";

export const OFFLINE_MS = 20_000;
export const LOAD_CPU = 85;
export const LOAD_MEM = 90;

export function deriveStatus(machine: Machine, now = Date.now()): MachineStatus {
  if (now - machine.lastSeen > OFFLINE_MS) return "offline";
  if (machine.frozenHint) return "frozen";
  if (machine.metricsLimited && machine.cpu == null && machine.memory == null) {
    return "limited";
  }
  const cpuLoad = machine.cpu != null && machine.cpu >= LOAD_CPU;
  const memLoad = machine.memory != null && machine.memory >= LOAD_MEM;
  if (cpuLoad || memLoad) return "under_load";
  return "online";
}

export function lastSeenLabel(lastSeen: number, now = Date.now()): string {
  const delta = Math.max(0, now - lastSeen);
  if (delta < 2_000) return "just now";
  if (delta < 60_000) return `${Math.round(delta / 1000)}s ago`;
  if (delta < 3_600_000) return `${Math.round(delta / 60_000)}m ago`;
  if (delta < 86_400_000) return `${Math.round(delta / 3_600_000)}h ago`;
  return `${Math.round(delta / 86_400_000)}d ago`;
}

export const STATUS_LABEL: Record<MachineStatus, string> = {
  online: "Online",
  under_load: "Under load",
  frozen: "Frozen",
  offline: "Offline",
  limited: "Limited",
};
