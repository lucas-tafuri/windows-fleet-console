"use client";

import { JobResults } from "@/components/job-results";
import { useFleet } from "@/hooks/use-fleet";
import { JOB_LABEL } from "@/lib/labels";

export default function JobsPage() {
  const { data, loading, error } = useFleet();

  return (
    <div className="flex min-h-full flex-col px-4 py-5 md:px-6">
      <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
        History
      </p>
      <h2 className="mt-1 text-2xl font-medium tracking-tight">Jobs</h2>
      <p className="mt-1 max-w-xl text-sm text-muted-foreground">
        Bulk actions across the fleet. Each result lists the path that ran
        (winget, registry, net use, Win32).
      </p>

      <div className="mt-6 grid gap-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading jobs…</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : data.jobs.length === 0 ? (
          <div className="rounded-xl border border-dashed border-white/10 px-6 py-14">
            <p className="font-mono text-[11px] tracking-[0.22em] text-primary uppercase">
              Empty
            </p>
            <h3 className="mt-2 text-lg font-medium">No jobs yet</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Select machines on Fleet and run software, drives, clean, or
              launch.
            </p>
          </div>
        ) : (
          data.jobs.map((job) => (
            <article
              key={job.id}
              className="rounded-xl border border-white/8 bg-card/70 p-4"
            >
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium">{JOB_LABEL[job.kind]}</p>
                  <p className="font-mono text-[11px] text-muted-foreground">
                    {new Date(job.createdAt).toLocaleString()} · {job.status}
                    {job.payload.package ? ` · ${job.payload.package}` : ""}
                    {job.payload.target ? ` · ${job.payload.target}` : ""}
                    {job.payload.letter ? ` · ${job.payload.letter}:` : ""}
                  </p>
                </div>
              </div>
              <JobResults job={job} />
            </article>
          ))
        )}
      </div>
    </div>
  );
}
