"use client";

import { useCallback, useRef, useState, useSyncExternalStore } from "react";
import type { CatalogApp, FleetSnapshot, Job, JobKind, JobPayload } from "@/lib/types";

const EMPTY: FleetSnapshot = {
  machines: [], jobs: [], software: [], softwareStatus: {}, pendingJoins: [],
  mapPrefs: { letter: "Z", unc: "", username: "", password: "", uncHistory: [] },
  demoActive: false, pinRequired: false, unlocked: true, unprotected: true, serverTime: 0,
};
let state = { data: EMPTY, loading: true, error: null as string | null, transport: "poll" as "sse" | "poll" };
const listeners = new Set<() => void>();
let stream: EventSource | undefined;
let timer: ReturnType<typeof setInterval> | undefined;
let pulling = false;
function publish(next: typeof state) { state = next; listeners.forEach((fn) => fn()); }
function accept(data: FleetSnapshot, transport: "sse" | "poll") {
  publish({ data, transport, loading: false, error: null });
  try { localStorage.setItem("fleet-peer-links", JSON.stringify((data.peers || []).map((p) => ({ name: p.member.hostname, url: p.member.dashboardUrl })))); } catch { /* storage optional */ }
}
async function pull() {
  if (pulling) return;
  pulling = true;
  try {
    const response = await fetch("/api/v2/fleet", { cache: "no-store", signal: AbortSignal.timeout(8_000) });
    if (!response.ok) throw new Error("This node is unavailable");
    accept(await response.json() as FleetSnapshot, "poll");
  } catch { publish({ ...state, loading: false, error: "Cannot reach this node. Open another peer below." }); }
  finally { pulling = false; }
}
function subscribe(listener: () => void) {
  listeners.add(listener);
  if (listeners.size === 1) {
    void pull();
    stream = new EventSource("/api/v2/events");
    stream.onmessage = (event) => { try { accept(JSON.parse(event.data) as FleetSnapshot, "sse"); } catch { void pull(); } };
    stream.onerror = () => { publish({ ...state, transport: "poll" }); void pull(); };
    timer = setInterval(() => { if (stream?.readyState !== EventSource.OPEN) void pull(); }, 2_000);
  }
  return () => { listeners.delete(listener); if (!listeners.size) { stream?.close(); stream = undefined; clearInterval(timer); timer = undefined; } };
}
export function submissionID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (v) => v.toString(16).padStart(2, "0")).join("");
}
export async function fleetRequest<T>(path: string, body: unknown, method = "POST"): Promise<T> {
  const response = await fetch(`/api/v2${path}`, { method, headers: { "Content-Type": "application/json", "X-Fleet-Request": "v2" }, body: JSON.stringify(body) });
  const result = await response.json() as T & { error?: string };
  if (!response.ok) throw new Error(result.error || "Request failed");
  await pull();
  return result;
}
export type DeliveryOptions = { delivery?: "online" | "queued"; expiresAt?: number; retryOf?: string };

export function useFleet() {
  const snapshot = useSyncExternalStore(subscribe, () => state, () => state);
  const [busy, setBusy] = useState(false);
  const pending = useRef<{ body: string; id: string } | null>(null);
  const submitJob = useCallback(async (kind: JobKind, machineIds: string[], payload: JobPayload = {}, options: DeliveryOptions = {}) => {
    const body = { kind, machineIds: [...machineIds].sort(), payload, delivery: options.delivery || "online", ...(options.expiresAt ? { expiresAt: options.expiresAt } : {}), ...(options.retryOf ? { retryOf: options.retryOf } : {}) };
    const serialized = JSON.stringify(body);
    if (!pending.current || pending.current.body !== serialized) pending.current = { body: serialized, id: submissionID() };
    setBusy(true);
    try {
      const result = await fleetRequest<{ job: Job }>("/jobs", { ...body, submissionId: pending.current.id });
      pending.current = null;
      return result.job;
    } finally { setBusy(false); }
  }, []);
  const mutate = useCallback(async <T,>(path: string, body: unknown, method?: string) => { setBusy(true); try { return await fleetRequest<T>(path, body, method); } finally { setBusy(false); } }, []);
  return { ...snapshot, busy, pull, submitJob,
    clearJobs: () => mutate("/jobs", {}, "DELETE"),
    addSoftware: async (input: { name: string; match?: string; wingetId?: string }) => (await mutate<{ item: CatalogApp }>("/software", input)).item,
    removeSoftware: (id: string) => mutate("/software", { id }, "DELETE"),
    decideJoin: (id: string, action: "approve" | "deny") => mutate(`/join/${encodeURIComponent(id)}`, { action }),
  };
}
