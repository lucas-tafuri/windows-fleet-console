"use client";

import { useState } from "react";
import { JobResults } from "@/components/job-results";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useFleet } from "@/hooks/use-fleet";
import { JOB_LABEL } from "@/lib/labels";
import { downloadReport } from "@/components/diagnostic-report";
import { cn } from "@/lib/utils";

const attention = ["error", "unknown", "expired", "unavailable", "acceptance_unknown", "held"];

export default function JobsPage() {
  const { data, loading, error, busy, clearJobs } = useFleet();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [clearError, setClearError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("all");
  const [machine, setMachine] = useState(() => new URLSearchParams(window.location.search).get("machine") || "");
  const [limit, setLimit] = useState(20);
  const filtered = data.jobs.filter((j) => {
    const results = Object.values(j.results);
    const matchesStatus = filter === "all" || (filter === "attention" ? results.some((r) => attention.includes(r.status)) : filter === "active" ? results.some((r) => ["queued", "running", "held"].includes(r.status)) : results.length > 0 && results.every((r) => r.status === "ok"));
    return matchesStatus && (!machine || j.machineIds.includes(machine)) && `${JOB_LABEL[j.kind]} ${j.payload.profile || ""} ${j.payload.package || ""} ${j.id} ${results.map((r) => `${r.hostname} ${r.message}`).join(" ")}`.toLowerCase().includes(search.toLowerCase());
  });

  async function onClear() {
    setClearError(null);
    try {
      await clearJobs();
      setConfirmOpen(false);
    } catch (err) {
      setClearError(err instanceof Error ? err.message : "Could not clear jobs");
    }
  }

  return (
    <div className="flex min-h-full flex-col px-4 py-5 md:px-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
            History
          </p>
          <h2 className="mt-1 text-2xl font-medium tracking-tight">Jobs</h2>
          <p className="mt-1 max-w-xl text-sm text-muted-foreground">
            Track delivery, review results, and follow up on work that needs attention.
          </p>
        </div>
        <Button
          variant="outline"
          disabled={busy || data.jobs.length === 0}
          onClick={() => setConfirmOpen(true)}
        >
          Clear history
        </Button>
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-3"><div className="flex gap-1">{[{ id: "all", label: "All activity" }, { id: "active", label: "In progress" }, { id: "attention", label: "Needs attention" }, { id: "complete", label: "Successful" }].map((f) => <button key={f.id} aria-pressed={filter === f.id} className={cn("fleet-filter", filter === f.id && "fleet-filter-active")} onClick={() => { setFilter(f.id); setLimit(20); }}>{f.label}</button>)}</div><input className="tool-input flex-1 min-w-40" aria-label="Search jobs" placeholder="Search machines, reports or jobs…" value={search} onChange={(e) => { setSearch(e.target.value); setLimit(20); }} /><select className="tool-input !w-auto" aria-label="Filter jobs by machine" value={machine} onChange={(e) => { setMachine(e.target.value); setLimit(20); }}><option value="">All machines</option>{data.machines.map((m) => <option key={m.id} value={m.id}>{m.hostname}</option>)}</select><Button variant="outline" disabled={!filtered.length} onClick={() => downloadReport("fleet-job-results.json", JSON.stringify(filtered.map((j) => ({ id: j.id, kind: j.kind, createdAt: j.createdAt, delivery: j.delivery, expiresAt: j.expiresAt, results: j.results })), null, 2))}>Export results</Button></div>
      <p className="mt-3 text-xs text-muted-foreground">{filtered.length} matching jobs</p>
      <div className="mt-4 grid gap-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading jobs…</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : filtered.length === 0 ? (
          <div className="rounded-xl border border-dashed border-white/10 px-6 py-14">
            <p className="font-mono text-[11px] tracking-[0.22em] text-primary uppercase">
              Empty
            </p>
            <h3 className="mt-2 text-lg font-medium">No matching jobs</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Select machines on Fleet and run software, drives, clean, or
              launch.
            </p>
          </div>
        ) : (
          filtered.slice(0, limit).map((job) => (
            <details
              key={job.id}
              className="rounded-xl border border-white/8 bg-card/70 p-4"
            >
              <summary className="cursor-pointer">
                <div>
                  <span className="text-sm font-medium">{JOB_LABEL[job.kind]}{job.payload.profile ? ` · ${job.payload.profile}` : ""}<span className="ml-3 text-xs text-muted-foreground">{Object.values(job.results).map((r) => r.hostname).join(", ")}</span></span>
                  <p className="font-mono text-[11px] text-muted-foreground">
                    {new Date(job.createdAt).toLocaleString()} · {job.status}
                    {job.payload.package ? ` · ${job.payload.package}` : ""}
                    {job.payload.target ? ` · ${job.payload.target}` : ""}
                    {job.payload.letter ? ` · ${job.payload.letter}:` : ""}
                  </p>
                  <p className="mt-2 text-xs text-muted-foreground"><span className="text-online">{Object.values(job.results).filter((r) => r.status === "ok").length} complete</span>{Object.values(job.results).some((r) => attention.includes(r.status)) && <span className="ml-3 text-load">{Object.values(job.results).filter((r) => attention.includes(r.status)).length} need attention</span>}{Object.values(job.results).some((r) => ["queued", "running"].includes(r.status)) && <span className="ml-3">{Object.values(job.results).filter((r) => ["queued", "running"].includes(r.status)).length} pending</span>}</p>
                </div>
              </summary>
              <div className="mt-4"><JobResults job={job} /></div>
            </details>
          ))
        )}
      </div>
      {filtered.length > limit && <Button variant="outline" className="mt-4 self-center" onClick={() => setLimit((v) => v + 20)}>Show 20 more</Button>}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Clear job history?</DialogTitle>
            <DialogDescription>
              This hides jobs on this dashboard. Execution identities and fleet
              history remain stored so old work cannot run again.
            </DialogDescription>
          </DialogHeader>
          {clearError ? (
            <p className="text-sm text-destructive">{clearError}</p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" disabled={busy} onClick={() => void onClear()}>
              {busy ? "Clearing…" : "Clear history"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
