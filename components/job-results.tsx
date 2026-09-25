"use client";

import type { Job } from "@/lib/types";
import { JOB_LABEL } from "@/lib/labels";
import { cn } from "@/lib/utils";

export function JobResults({ job }: { job: Job }) {
  const rows = Object.values(job.results);
  return (
    <div className="job-results flex min-w-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium">{JOB_LABEL[job.kind]}</p>
        <p className="max-w-32 truncate font-mono text-[10px] text-muted-foreground" title={job.id}>{job.id}</p>
      </div>
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
