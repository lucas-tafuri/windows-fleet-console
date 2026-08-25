"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { CatalogApp, FleetSnapshot, Job, JobKind, JobPayload } from "@/lib/types";

const EMPTY: FleetSnapshot = {
  machines: [],
  jobs: [],
  software: [],
  softwareStatus: {},
  demoActive: false,
  pinRequired: false,
  unlocked: true,
  unprotected: true,
  serverTime: Date.now(),
};

export function useFleet() {
  const [data, setData] = useState<FleetSnapshot>(EMPTY);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [transport, setTransport] = useState<"ws" | "poll">("poll");
  const [busy, setBusy] = useState(false);
  const wsTried = useRef(false);

  const pull = useCallback(async () => {
    try {
      const res = await fetch("/api/fleet", { cache: "no-store" });
      if (!res.ok) throw new Error(`Fleet ${res.status}`);
      const json = (await res.json()) as FleetSnapshot;
      setData({
        ...json,
        software: json.software || [],
        softwareStatus: json.softwareStatus || {},
      });
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Cannot reach console");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let closed = false;
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let ws: WebSocket | undefined;

    const startPoll = () => {
      if (pollTimer) return;
      setTransport("poll");
      void pull();
      pollTimer = setInterval(() => void pull(), 2000);
    };

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    try {
      ws = new WebSocket(`${proto}//${window.location.host}/api/ui/ws`);
      ws.onopen = () => {
        wsTried.current = true;
      };
      ws.onmessage = (ev) => {
        try {
          const json = JSON.parse(String(ev.data)) as FleetSnapshot;
          if (closed) return;
          setData({
            ...json,
            software: json.software || [],
            softwareStatus: json.softwareStatus || {},
          });
          setLoading(false);
          setError(null);
          setTransport("ws");
          if (pollTimer) {
            clearInterval(pollTimer);
            pollTimer = undefined;
          }
        } catch {
          /* ignore malformed frames */
        }
      };
      ws.onerror = () => {
        if (!closed) startPoll();
      };
      ws.onclose = () => {
        if (!closed) startPoll();
      };
    } catch {
      startPoll();
    }

    const fallback = setTimeout(() => {
      if (!closed && transport !== "ws") startPoll();
    }, 1200);

    return () => {
      closed = true;
      clearTimeout(fallback);
      if (pollTimer) clearInterval(pollTimer);
      ws?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pull]);

  const unlock = useCallback(async (pin: string) => {
    const res = await fetch("/api/pin", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ pin }),
    });
    const json = (await res.json()) as { ok?: boolean; error?: string };
    if (!res.ok || !json.ok) throw new Error(json.error || "Wrong PIN");
    await pull();
  }, [pull]);

  const submitJob = useCallback(
    async (kind: JobKind, machineIds: string[], payload: JobPayload = {}) => {
      setBusy(true);
      try {
        const res = await fetch("/api/jobs", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ kind, machineIds, payload }),
        });
        const json = (await res.json()) as { job?: Job; error?: string };
        if (!res.ok || !json.job) throw new Error(json.error || "Job failed");
        await pull();
        return json.job;
      } finally {
        setBusy(false);
      }
    },
    [pull]
  );

  const clearJobs = useCallback(async () => {
    setBusy(true);
    try {
      const res = await fetch("/api/jobs", { method: "DELETE" });
      const json = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) throw new Error(json.error || "Could not clear jobs");
      await pull();
    } finally {
      setBusy(false);
    }
  }, [pull]);

  const addSoftware = useCallback(
    async (input: { name: string; match?: string; wingetId?: string }) => {
      setBusy(true);
      try {
        const res = await fetch("/api/software", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify(input),
        });
        const json = (await res.json()) as { item?: CatalogApp; error?: string };
        if (!res.ok || !json.item) throw new Error(json.error || "Could not add software");
        await pull();
        return json.item;
      } finally {
        setBusy(false);
      }
    },
    [pull]
  );

  const removeSoftware = useCallback(
    async (id: string) => {
      setBusy(true);
      try {
        const res = await fetch("/api/software", {
          method: "DELETE",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ id }),
        });
        const json = (await res.json().catch(() => ({}))) as { error?: string };
        if (!res.ok) throw new Error(json.error || "Could not remove software");
        await pull();
      } finally {
        setBusy(false);
      }
    },
    [pull]
  );

  return {
    data,
    loading,
    error,
    transport,
    busy,
    pull,
    unlock,
    submitJob,
    clearJobs,
    addSoftware,
    removeSoftware,
  };
}
