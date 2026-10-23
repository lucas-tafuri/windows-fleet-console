"use client";

import type { Job } from "@/lib/types";
import { JOB_LABEL } from "@/lib/labels";
import { cn } from "@/lib/utils";
import { useRef, useState } from "react";
import { fleetRequest, submissionID } from "@/hooks/use-fleet";

export function JobResults({ job }: { job: Job }) {
  const rows = Object.values(job.results);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const retryIDs = useRef<Record<string, string>>({});
  async function retry(id: string) {
    if (!window.confirm("Retry as a new execution? If the previous outcome is unknown, inspect the PC first to avoid repeating a side effect.")) return;
    retryIDs.current[id] ||= submissionID();
    setBusy(id); setError("");
    try { await fleetRequest("/retry", { commandId: id, submissionId: retryIDs.current[id] }); delete retryIDs.current[id]; }
    catch (e) { setError(e instanceof Error ? e.message : "Retry failed"); }
    finally { setBusy(""); }
  }
  return (
    <div className="job-results flex min-w-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium">{JOB_LABEL[job.kind]}</p>
        <p className="max-w-32 truncate font-mono text-[10px] text-muted-foreground" title={job.id}>{job.id}</p>
      </div>
      {job.delivery === "queued" ? <p className="text-xs text-muted-foreground">Queued explicitly · expires {job.expiresAt ? new Date(job.expiresAt).toLocaleString() : "unknown"}</p> : null}
      {job.historical ? <p className="text-xs text-muted-foreground">Imported history · no automatic execution</p> : null}
      {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
      <ul className="flex flex-col gap-1.5">
        {rows.map((row) => (
          <li
            key={row.machineId}
            className="rounded-xl border border-white/8 bg-black/10 px-4 py-3 transition-opacity duration-150"
          >
            <div className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate text-xs font-semibold" title={row.hostname}>{row.hostname}</span>
              <span
                className={cn(
                  "font-mono text-[10px] tracking-wide uppercase",
                  row.status === "ok" && "text-online",
                  row.status === "error" && "text-frozen",
                  row.status === "running" && "text-load",
                  row.status === "queued" && "text-muted-foreground"
                )}
              >
                {row.status === "ok" ? "Complete" : row.status}
              </span>
            </div>
            <p className="mt-2 break-words text-xs leading-relaxed text-muted-foreground">{row.message}</p>
            {row.replicaCount !== undefined && !job.historical ? <p className="mt-1 text-[11px] text-muted-foreground">{row.replicaCount ? `${row.replicaCount} additional confirmed ${row.replicaCount === 1 ? "copy" : "copies"}` : "Local record only · no confirmed backup copy"}</p> : null}
            {row.commandId && ["unknown", "error", "expired", "unavailable", "acceptance_unknown"].includes(row.status) ? <button className="mt-2 text-xs text-primary underline" disabled={Boolean(busy)} onClick={() => void retry(row.commandId!)}>{busy === row.commandId ? "Submitting…" : "Review and retry"}</button> : null}
            {row.via ? (
              <p className="mt-0.5 font-mono text-[10px] text-primary/80">
                via {row.via}
              </p>
            ) : null}
            {row.output ? (
              <details className="job-log mt-3">
                <summary className="cursor-pointer text-[11px] text-muted-foreground">View output</summary>
                <pre className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap break-all rounded-lg bg-black/20 p-3 font-mono text-[10px] leading-relaxed text-foreground/70">{row.output}</pre>
              </details>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
