"use client";

import type { Job } from "@/lib/types";
import { JOB_LABEL } from "@/lib/labels";
import { cn } from "@/lib/utils";

export function JobResults({ job }: { job: Job }) {
  const rows = Object.values(job.results);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium">{JOB_LABEL[job.kind]}</p>
        <p className="font-mono text-[11px] text-muted-foreground">{job.id}</p>
      </div>
      <ul className="flex flex-col gap-1.5">
        {rows.map((row) => (
          <li
            key={row.machineId}
            className="rounded-lg border border-white/8 bg-black/20 px-3 py-2 transition-opacity duration-150"
          >
            <div className="flex items-center justify-between gap-2">
              <span className="font-mono text-xs">{row.hostname}</span>
              <span
                className={cn(
                  "font-mono text-[10px] tracking-wide uppercase",
                  row.status === "ok" && "text-online",
                  row.status === "error" && "text-frozen",
                  row.status === "running" && "text-load",
                  row.status === "queued" && "text-muted-foreground"
                )}
              >
                {row.status}
              </span>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{row.message}</p>
            {row.via ? (
              <p className="mt-0.5 font-mono text-[10px] text-primary/80">
                via {row.via}
              </p>
            ) : null}
            {row.output ? (
              <pre className="mt-2 max-h-24 overflow-auto font-mono text-[10px] leading-relaxed text-foreground/70">
                {row.output}
              </pre>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
