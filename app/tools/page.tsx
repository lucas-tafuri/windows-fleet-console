"use client";

import { useState } from "react";
import { Wrench, Play, CheckCircle2 } from "lucide-react";
import { useFleet } from "@/hooks/use-fleet";
import { Button } from "@/components/ui/button";
import { JobResults } from "@/components/job-results";
import { REPORTS } from "@/components/diagnostic-report";
import type { Job } from "@/lib/types";
import { cn } from "@/lib/utils";

export default function ToolsPage() {
  const { data, loading, error, busy, submitJob } = useFleet();
  const [profile, setProfile] = useState<string>("system");
  const [selected, setSelected] = useState<string[]>(() => new URLSearchParams(window.location.search).getAll("machine"));
  const [failure, setFailure] = useState("");
  const [lastJob, setLastJob] = useState<Job | null>(null);
  const machines = data.machines.filter((m) => (m.transport === "mesh" && !m.historical));
  const supported = machines.filter((m) => m.status !== "offline" && m.kinds?.includes("diagnostics"));
  const targets = selected.filter((id) => supported.some((m) => m.id === id));
  const report = REPORTS.find((r) => r.id === profile)!;
  const recent = data.jobs.filter((j) => j.kind === "diagnostics").slice(0, 8);
  const current = lastJob && (data.jobs.find((j) => j.id === lastJob.id) || lastJob);
  async function run() {
    setFailure("");
    try { setLastJob(await submitJob("diagnostics", targets, { profile })); }
    catch (e) { setFailure(e instanceof Error ? e.message : "Could not request report"); }
  }
  return <main className="workspace-page">
    <header className="workspace-header"><div><p className="fleet-eyebrow">WORKSPACE / TOOLS</p><h2>Understand every PC<span className="text-primary">.</span></h2><p>Collect useful Windows reports from one machine or the whole fleet.</p></div><span className="tool-readonly"><CheckCircle2 className="size-4" /> Read-only tools</span></header>
    <div className="tools-layout">
      <section aria-label="Choose a report"><div className="section-heading"><Wrench className="size-4" /><h3>1. Choose a report</h3></div><div className="tool-grid">{REPORTS.map((r) => <button type="button" key={r.id} aria-pressed={profile === r.id} className={cn("tool-card", profile === r.id && "tool-card-active")} onClick={() => setProfile(r.id)}><span className="tool-symbol" aria-hidden="true">{r.icon}</span><strong>{r.name}</strong><p>{r.description}</p></button>)}</div><p className="mt-4 text-xs leading-relaxed text-muted-foreground">Reports run on the selected PCs and are saved in Jobs. Each is a point-in-time snapshot, not a live process monitor. Existing software, drive and cleanup actions remain on Fleet.</p></section>
      <section className="tool-targets" aria-label="Choose report targets"><div className="section-heading"><h3>2. Choose machines</h3><button className="ml-auto text-xs text-primary" onClick={() => setSelected(targets.length === supported.length ? [] : supported.map((m) => m.id))}>{targets.length === supported.length && supported.length ? "Clear" : "Select reachable"}</button></div>
        {loading ? <p>Loading machines…</p> : error ? <p role="alert" className="text-destructive">{error}</p> : machines.map((m) => { const available = supported.some((s) => s.id === m.id); return <label key={m.id} className={cn("tool-target", !available && "opacity-50")}><input type="checkbox" disabled={!available || busy} checked={targets.includes(m.id)} onChange={(e) => setSelected((old) => e.target.checked ? [...old, m.id] : old.filter((id) => id !== m.id))} /><span className="min-w-0 flex-1"><strong>{m.hostname}</strong><small>{!available ? m.status === "offline" ? "Unreachable from this node" : "Node update required" : m.user || "No user signed in"}</small></span><span className={available ? "text-online" : "text-muted-foreground"}>●</span></label>; })}
        <div className="border-t border-white/10 pt-4 mt-4"><p className="mb-3 text-xs text-muted-foreground">{report.name} · {targets.length} selected · immediate delivery</p><Button className="w-full" disabled={!targets.length || busy || Boolean(error)} onClick={() => void run()}><Play className="size-4" />{busy ? "Requesting reports…" : `Run on ${targets.length} ${targets.length === 1 ? "machine" : "machines"}`}</Button>{failure && <p role="alert" className="mt-3 text-sm text-destructive">{failure}</p>}</div>
      </section>
    </div>
    <section className="mt-8" aria-label="Reports"><div className="section-heading"><h3>{current ? "Requested report" : "Recent reports"}</h3><a href="/jobs" className="ml-auto text-xs text-primary">All activity →</a></div>{current ? <article className="report-card"><h3 className="mb-3 font-medium">{REPORTS.find((r) => r.id === current.payload.profile)?.name}</h3><JobResults job={current} /></article> : recent.length ? recent.map((job) => <details className="report-card mb-3" key={job.id}><summary className="cursor-pointer text-sm">{REPORTS.find((r) => r.id === job.payload.profile)?.name} · {new Date(job.createdAt).toLocaleString()} · {job.machineIds.length} machines</summary><div className="mt-4"><JobResults job={job} /></div></details>) : <div className="report-card text-sm text-muted-foreground">Your reports will appear here. Start with System overview or Disk space.</div>}</section>
  </main>;
}
