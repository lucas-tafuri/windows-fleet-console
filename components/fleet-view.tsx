"use client";

import { useMemo, useState } from "react";
import {
  Search,
  Monitor,
  Plus,
  FolderDown,
  HardDrive,
  Package,
  RefreshCw,
  Rocket,
  Trash2,
  Unplug,
} from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ActionSheet, type ActionKey } from "@/components/action-sheet";
import { Meter } from "@/components/meter";
import { StatusLamp } from "@/components/status-lamp";
import { JobResults } from "@/components/job-results";
import { useFleet } from "@/hooks/use-fleet";
import { STATUS_LABEL } from "@/lib/status";
import type { Job, MachineStatus, MachineView } from "@/lib/types";
import { cn } from "@/lib/utils";

const FILTERS: Array<{ id: "all" | MachineStatus; label: string }> = [
  { id: "all", label: "All" },
  { id: "online", label: "Online" },
  { id: "under_load", label: "Load" },
  { id: "frozen", label: "Frozen" },
  { id: "offline", label: "Offline" },
  { id: "limited", label: "Limited" },
];

export function FleetView() {
  const { data, loading, error, transport, busy, submitJob } = useFleet();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | MachineStatus>("all");
  const [selected, setSelected] = useState<string[]>([]);
  const [action, setAction] = useState<ActionKey | null>(null);
  const [lastJob, setLastJob] = useState<Job | null>(null);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return data.machines.filter((m) => (filter === "all" || m.status === filter) &&
      `${m.hostname} ${m.user} ${m.os}`.toLowerCase().includes(query));
  }, [data.machines, filter, search]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: data.machines.length };
    for (const m of data.machines) c[m.status] = (c[m.status] || 0) + 1;
    return c;
  }, [data.machines]);

  const liveJob = useMemo(() => {
    if (lastJob) {
      return data.jobs.find((j) => j.id === lastJob.id) || lastJob;
    }
    return data.jobs[0] || null;
  }, [data.jobs, lastJob]);

  const allFilteredSelected =
    filtered.length > 0 && filtered.every((m) => selected.includes(m.id));

  function toggleAll(next: boolean) {
    if (next) setSelected((cur) => Array.from(new Set([...cur, ...filtered.map((m) => m.id)])));
    else setSelected((cur) => cur.filter((id) => !filtered.some((m) => m.id === id)));
  }

  function toggleOne(id: string, next: boolean) {
    setSelected((cur) =>
      next ? Array.from(new Set([...cur, id])) : cur.filter((x) => x !== id)
    );
  }

  async function onSubmit(kind: Parameters<typeof submitJob>[0], payload: Parameters<typeof submitJob>[2]) {
    const job = await submitJob(kind, selected, payload);
    setLastJob(job);
  }

  return (
    <div className="flex min-h-full min-w-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col">
        {data.demoActive ? (
          <div className="border-b border-primary/25 bg-primary/10 px-4 py-2 text-center text-xs text-primary md:text-left">
            Simulated PCs are shown so you can exercise every action. They
            hide automatically when a real agent connects.
          </div>
        ) : null}
        {data.unprotected ? (
          <div className="border-b border-white/8 bg-white/3 px-4 py-1.5 text-center text-[11px] text-muted-foreground md:text-left">
            Dashboard PIN is unset — this console is open on the local network.
            Set <span className="font-mono text-foreground/80">DASHBOARD_PIN</span> to
            lock it.
          </div>
        ) : null}

        <header className="fleet-heading">
          <div>
            <p className="fleet-eyebrow">WORKSPACE / OVERVIEW</p>
            <h2 className="mt-2 text-3xl font-semibold tracking-tight">Your fleet<span className="text-primary">.</span></h2>
            <p className="mt-2 text-sm text-muted-foreground">Every machine. One clear view.</p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <span className={cn("fleet-connection", error ? "text-frozen" : "text-online")}>
              <span className="size-1.5 rounded-full bg-current" />
              {error ? "Disconnected" : loading ? "Connecting" : "Live updates"}<span className="sr-only">{transport}</span>
            </span>
            <Link href="/enroll" className="fleet-add"><Plus className="size-4" /> Add machine</Link>
          </div>
        </header>
        <div className="fleet-overview" aria-label="Fleet summary">
          {[
            { label: "Total machines", value: counts.all, note: "In your workspace", color: "text-foreground" },
            { label: "Connected", value: counts.all - (counts.offline || 0), note: "Reporting to the console", color: "text-online" },
            { label: "Needs attention", value: (counts.under_load || 0) + (counts.frozen || 0) + (counts.limited || 0), note: "Load, frozen or limited", color: "text-load" },
            { label: "Offline", value: counts.offline || 0, note: "Not currently reporting", color: "text-muted-foreground" },
          ].map((stat) => <div className="fleet-stat" key={stat.label}>
            <p className="text-xs font-medium text-muted-foreground">{stat.label}</p>
            <p className={cn("mt-2 text-3xl font-semibold tabular tracking-tight", stat.color)}>{loading || error ? "—" : stat.value}</p>
            <p className="mt-2 text-[11px] text-muted-foreground">{stat.note}</p>
          </div>)}
        </div>
        <div className="fleet-toolbar">
          <div className="flex flex-wrap gap-1" aria-label="Filter machines">
            {FILTERS.map((f) => <button key={f.id} type="button" onClick={() => setFilter(f.id)} aria-pressed={filter === f.id}
              className={cn("fleet-filter", filter === f.id && "fleet-filter-active")}>
              {f.label}<span className="tabular">{counts[f.id] || 0}</span>
            </button>)}
          </div>
          <label className="fleet-search"><Search className="size-4 shrink-0" />
            <input aria-label="Search machines" placeholder="Search machines or users…" value={search} onChange={(e) => setSearch(e.target.value)} />
          </label>
        </div>
        <div className="fleet-list-heading">
          <label className="flex cursor-pointer items-center gap-2"><Checkbox checked={allFilteredSelected} onCheckedChange={(v) => toggleAll(Boolean(v))} aria-label="Select all visible machines" />Select all</label>
          <span>{filtered.length} machines{selected.length > 0 ? ` · ${selected.length} selected` : ""}</span>
        </div>
        <div className="min-h-0 flex-1 px-4 pb-44 md:px-6">
          {loading ? (
            <EmptyState title="Listening for machines">
              Heartbeats arrive every few seconds.
            </EmptyState>
          ) : error ? (
            <EmptyState title="Console unreachable">{error}</EmptyState>
          ) : filtered.length === 0 ? (
            <EmptyState title={data.machines.length === 0 ? "No machines yet" : "Nothing in this filter"}>
              {data.machines.length === 0
                ? "Open Enroll, copy the agent command, and run it on a Windows PC."
                : "Try a different search or status filter."}
            </EmptyState>
          ) : (
            <div className="fleet-machine-grid">{filtered.map((machine) => (
              <MachineCard key={machine.id} machine={machine} checked={selected.includes(machine.id)} onChecked={(v) => toggleOne(machine.id, v)} />
            ))}</div>
          )}
          {liveJob ? <details className="fleet-job-panel mt-6">
            <summary className="cursor-pointer px-5 py-4 text-sm font-medium">Latest activity <span className="ml-2 text-xs text-muted-foreground">View job results</span></summary>
            <div className="border-t border-white/8 p-5"><JobResults job={liveJob} /></div>
          </details> : null}
        </div>

        <div
          hidden={selected.length === 0}
          className={cn(
            "fixed right-0 bottom-16 left-0 z-20 border-t border-white/8 bg-sidebar/95 px-3 py-2 backdrop-blur-md transition-[opacity,transform] duration-200 ease-out md:bottom-0 md:left-52 md:px-6",
            selected.length === 0
              ? "pointer-events-none translate-y-4 opacity-0"
              : "opacity-100"
          )}
        >
          <div className="flex flex-wrap items-center gap-2">
            <span className="mr-2 font-mono text-[11px] text-primary">
              {selected.length} selected
            </span>
            <DockBtn icon={Package} label="Software" onClick={() => setAction("package")} />
            <DockBtn icon={HardDrive} label="Map" onClick={() => setAction("map")} />
            <DockBtn icon={Unplug} label="Unmap" onClick={() => setAction("unmap")} />
            <DockBtn icon={FolderDown} label="Downloads" onClick={() => setAction("clean")} />
            <DockBtn icon={Trash2} label="Recycle" onClick={() => setAction("recycle")} />
            <DockBtn icon={Rocket} label="Launch" onClick={() => setAction("launch")} />
            <DockBtn icon={RefreshCw} label="Update" onClick={() => setAction("update")} />
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setSelected([])}
              className="ml-auto"
            >
              Clear
            </Button>
          </div>
        </div>
      </div>


      <ActionSheet
        open={action !== null}
        onOpenChange={(o) => {
          if (!o) setAction(null);
        }}
        action={action}
        selectedCount={selected.length}
        busy={busy}
        lastJob={liveJob}
        software={data.software || []}
        mapPrefs={data.mapPrefs}
        onSubmit={onSubmit}
      />
    </div>
  );
}

function DockBtn({
  icon: Icon,
  label,
  onClick,
}: {
  icon: typeof Package;
  label: string;
  onClick: () => void;
}) {
  return (
    <Button variant="outline" size="sm" onClick={onClick}>
      <Icon />
      {label}
    </Button>
  );
}

function MachineCard({ machine, checked, onChecked }: {
  machine: MachineView; checked: boolean; onChecked: (next: boolean) => void;
}) {
  const offline = machine.status === "offline";
  return <button type="button" aria-pressed={checked} aria-label={`Select ${machine.hostname}`}
    onClick={() => onChecked(!checked)} className={cn("fleet-machine", checked && "fleet-machine-selected")}>
    <div className="flex items-start gap-3">
      <span className={cn("fleet-machine-icon", offline && "opacity-50")}><Monitor className="size-5" /></span>
      <div className="min-w-0 flex-1">
        <h3 className="truncate text-base font-semibold tracking-tight" title={machine.hostname}>{machine.hostname}</h3>
        <p className="mt-1 truncate text-xs text-muted-foreground">{machine.user || "No user signed in"}</p>
      </div>
      <span className={cn("fleet-selection", checked && "fleet-selection-on")} aria-hidden="true">{checked ? "✓" : ""}</span>
    </div>
    <div className="mt-5 flex items-center justify-between gap-2">
      <span className={cn("fleet-status", `fleet-status-${machine.status}`)}><StatusLamp status={machine.status} />{STATUS_LABEL[machine.status]}</span>
      <span className="text-[11px] text-muted-foreground">{machine.demo ? "Demo machine" : machine.lastSeenLabel}</span>
    </div>
    <div className="fleet-card-metrics" title={offline ? "Machine offline — live metrics unavailable" : "Live resource usage"}>
      <Meter label="CPU" value={offline ? null : machine.cpu} />
      <Meter label="RAM" value={offline ? null : machine.memory} warnAt={90} />
      <Meter label="GPU" value={offline ? null : machine.gpu ?? null} />
    </div>
    <div className="mt-4 flex items-center justify-between gap-3 text-[11px] text-muted-foreground">
      <span className="truncate">{machine.os || "Windows"}</span>
      <span className="shrink-0">{offline ? "Awaiting connection" : machine.mappedDrives.length ? `${machine.mappedDrives.length} mapped drives` : "Agent connected"}</span>
    </div>
  </button>;
}

function EmptyState({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-[40vh] flex-col items-start justify-center rounded-xl border border-dashed border-white/10 px-6 py-16">
      <p className="font-mono text-[11px] tracking-[0.22em] text-primary uppercase">
        Empty
      </p>
      <h3 className="mt-2 text-xl font-medium">{title}</h3>
      <p className="mt-2 max-w-md text-sm text-muted-foreground">{children}</p>
    </div>
  );
}
