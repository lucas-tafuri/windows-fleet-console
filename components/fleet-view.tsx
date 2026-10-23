"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
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
  List,
  LayoutGrid,
} from "lucide-react";
import Link from "@/components/navigation";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ActionSheet, type ActionKey } from "@/components/action-sheet";
import { Meter } from "@/components/meter";
import { StatusLamp } from "@/components/status-lamp";
import { fleetCapacity } from "@/lib/fleet-layout";
import { useFleet } from "@/hooks/use-fleet";
import { STATUS_LABEL } from "@/lib/status";
import type { Job, MachineStatus, MachineView } from "@/lib/types";
import { cn } from "@/lib/utils";
import { MachineDetails } from "@/components/machine-details";

const FILTERS: Array<{ id: "all" | MachineStatus; label: string }> = [
  { id: "all", label: "All" },
  { id: "online", label: "Online" },
  { id: "under_load", label: "Load" },
  { id: "frozen", label: "Frozen" },
  { id: "offline", label: "Unreachable" },
  { id: "limited", label: "Limited" },
];

export function FleetView() {
  const { data, loading, error, transport, busy, submitJob } = useFleet();
  const contentRef = useRef<HTMLDivElement>(null);
  const [capacity, setCapacity] = useState(8);
  const [page, setPage] = useState(0);
  useEffect(() => {
    const element = contentRef.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      setCapacity(fleetCapacity(entry.contentRect.width, entry.contentRect.height));
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | MachineStatus>("all");
  const [selected, setSelected] = useState<string[]>([]);
  const [action, setAction] = useState<ActionKey | null>(null);
  const [lastJob, setLastJob] = useState<Job | null>(null);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [showHistory, setShowHistory] = useState(false);
  const [layout, setLayout] = useState<"cards" | "list">(() => {
    try { return localStorage.getItem("fleet-layout") === "cards" ? "cards" : "list"; } catch { return "list"; }
  });
  const machines = useMemo(() => data.machines.filter((m) => showHistory || m.transport === "mesh" || m.demo), [data.machines, showHistory]);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return machines.filter((m) => (filter === "all" || m.status === filter) &&
      `${m.hostname} ${m.user} ${m.os}`.toLowerCase().includes(query));
  }, [machines, filter, search]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: machines.length };
    for (const m of machines) c[m.status] = (c[m.status] || 0) + 1;
    return c;
  }, [machines]);

  const liveJob = useMemo(() => {
    if (lastJob) {
      return data.jobs.find((j) => j.id === lastJob.id) || lastJob;
    }
    return data.jobs[0] || null;
  }, [data.jobs, lastJob]);

  const pageSize = layout === "list" ? 50 : capacity;
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const visibleMachines = filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize);
  const selectable = visibleMachines.filter((m) => m.transport === "mesh");

  const allFilteredSelected =
    selectable.length > 0 && selectable.every((m) => selected.includes(m.id));

  function toggleAll(next: boolean) {
    if (next) setSelected((cur) => Array.from(new Set([...cur, ...selectable.map((m) => m.id)])));
    else setSelected((cur) => cur.filter((id) => !visibleMachines.some((m) => m.id === id)));
  }

  function toggleOne(id: string, next: boolean) {
    setSelected((cur) =>
      next ? Array.from(new Set([...cur, id])) : cur.filter((x) => x !== id)
    );
  }

  async function onSubmit(kind: Parameters<typeof submitJob>[0], payload: Parameters<typeof submitJob>[2], options?: Parameters<typeof submitJob>[3]) {
    const job = await submitJob(kind, selected, payload, options);
    setLastJob(job);
  }

  return (
    <div className="fleet-screen">
      <div className="fleet-screen-inner">
        {data.demoActive ? (
          <div className="border-b border-primary/25 bg-primary/10 px-4 py-2 text-center text-xs text-primary md:text-left">
            Simulated PCs are shown so you can exercise every action. They
            hide automatically when a real agent connects.
          </div>
        ) : null}
        {data.unprotected ? (
          <div className="border-b border-white/8 bg-white/3 px-4 py-1.5 text-center text-[11px] text-muted-foreground md:text-left">
            Open on your local network · Commands run only after the target PC accepts them.
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
            { label: "Unreachable", value: counts.offline || 0, note: "From this dashboard", color: "text-muted-foreground" },
          ].map((stat) => <div className="fleet-stat" key={stat.label}>
            <p className="text-xs font-medium text-muted-foreground">{stat.label}</p>
            <p className={cn("mt-2 text-3xl font-semibold tabular tracking-tight", stat.color)}>{loading || error ? "—" : stat.value}</p>
            <p className="mt-2 text-[11px] text-muted-foreground">{stat.note}</p>
          </div>)}
        </div>
        <div className="fleet-toolbar">
          <div className="flex flex-wrap gap-1" aria-label="Filter machines">
            {FILTERS.map((f) => <button key={f.id} type="button" onClick={() => { setFilter(f.id); setPage(0); }} aria-pressed={filter === f.id}
              className={cn("fleet-filter", filter === f.id && "fleet-filter-active")}>
              {f.label}<span className="tabular">{counts[f.id] || 0}</span>
            </button>)}
          </div>
          <label className="fleet-search"><Search className="size-4 shrink-0" />
            <input aria-label="Search machines" placeholder="Search machines or users…" value={search} onChange={(e) => { setSearch(e.target.value); setPage(0); }} />
          </label>
        </div>
        <div className="fleet-list-heading">
          <label className="flex cursor-pointer items-center gap-2"><Checkbox checked={allFilteredSelected} onCheckedChange={(v) => toggleAll(Boolean(v))} aria-label="Select machines on this page" />Select page</label>
          <div className="flex flex-wrap items-center gap-3"><label className="flex items-center gap-2"><input type="checkbox" checked={showHistory} onChange={(e) => { setShowHistory(e.target.checked); setPage(0); }} />Include history</label><span>{filtered.length} machines{selected.length > 0 ? ` · ${selected.length} selected` : ""}</span>{(["list", "cards"] as const).map((mode) => <button key={mode} aria-label={`${mode === "list" ? "List" : "Card"} view`} aria-pressed={layout === mode} className={cn("rounded p-1.5", layout === mode && "bg-primary/15 text-primary")} onClick={() => { setLayout(mode); setPage(0); try { localStorage.setItem("fleet-layout", mode); } catch { /* optional */ } }}>{mode === "list" ? <List className="size-4" /> : <LayoutGrid className="size-4" />}</button>)}</div>
        </div>
        <div ref={contentRef} className="fleet-content">
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
            layout === "list" ? <div className="fleet-table-wrap"><table className="fleet-table machine-table"><thead><tr><th><span className="sr-only">Select</span></th><th>Machine / user</th><th>Status</th><th>CPU</th><th>RAM</th><th>GPU</th><th>Drives</th><th><span className="sr-only">Details</span></th></tr></thead><tbody>{visibleMachines.map((m) => <tr key={m.id} className={selected.includes(m.id) ? "bg-primary/5" : ""}><td><Checkbox disabled={m.transport !== "mesh"} aria-label={`Select ${m.hostname}`} checked={selected.includes(m.id)} onCheckedChange={(v) => toggleOne(m.id, Boolean(v))} /></td><td><button className="text-left" onClick={() => setDetailId(m.id)}><strong className="text-foreground">{m.hostname}</strong>{m.id === data.nodeId && <span className="ml-2 text-[10px] text-primary">THIS NODE</span>}<span className="block mt-1 text-xs text-muted-foreground">{m.user || "No user signed in"}</span></button></td><td><span className={cn("fleet-status", `fleet-status-${m.status}`)}><StatusLamp status={m.status} />{m.transport !== "mesh" ? "Historical" : STATUS_LABEL[m.status]}</span></td>{[m.cpu, m.memory, m.gpu].map((value, i) => <td key={i} className={value != null && value >= 90 ? "text-load" : ""}>{m.status === "offline" || value == null ? "—" : `${Math.round(value)}%`}</td>)}<td>{m.mappedDrives.length || "—"}</td><td><button className="text-primary" aria-label={`Details for ${m.hostname}`} onClick={() => setDetailId(m.id)}>Details →</button></td></tr>)}</tbody></table></div> : <div className="fleet-machine-grid">{visibleMachines.map((machine) => (
              <MachineCard key={machine.id} machine={machine} checked={selected.includes(machine.id)} onChecked={(v) => toggleOne(machine.id, v)} onDetails={() => setDetailId(machine.id)} />
            ))}</div>
          )}
        </div>
        <div className="fleet-pagination">
          <Link href="/jobs" className="text-muted-foreground hover:text-primary">View activity →</Link>
          <div className="flex items-center gap-3">
            <span className="text-muted-foreground">{filtered.length ? `${currentPage * pageSize + 1}–${Math.min((currentPage + 1) * pageSize, filtered.length)} of ${filtered.length}` : "0 machines"}</span>
            {pageCount > 1 ? <>
              <Button variant="outline" size="icon-sm" aria-label="Previous machines" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}><ChevronLeft /></Button>
              <span>Page {currentPage + 1} / {pageCount}</span>
              <Button variant="outline" size="icon-sm" aria-label="Next machines" disabled={currentPage === pageCount - 1} onClick={() => setPage(currentPage + 1)}><ChevronRight /></Button>
            </> : null}
          </div>
        </div>

        <div
          hidden={selected.length === 0}
          className="fleet-action-dock"
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


      <MachineDetails machine={data.machines.find((m) => m.id === detailId)} data={data} onClose={() => setDetailId(null)} />
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

function MachineCard({ machine, checked, onChecked, onDetails }: {
  machine: MachineView; checked: boolean; onChecked: (next: boolean) => void; onDetails: () => void;
}) {
  const offline = machine.status === "offline";
  return <article className={cn("fleet-machine", checked && "fleet-machine-selected")}>
    <div className="flex items-start gap-3">
      <span className={cn("fleet-machine-icon", offline && "opacity-50")}><Monitor className="size-5" /></span>
      <div className="min-w-0 flex-1">
        <button onClick={onDetails} className="text-left hover:text-primary"><h3 className="truncate text-base font-semibold tracking-tight" title={machine.hostname}>{machine.hostname}</h3></button>
        <p className="mt-1 truncate text-xs text-muted-foreground">{machine.user || "No user signed in"}</p>
      </div>
      <Checkbox disabled={machine.transport !== "mesh"} aria-label={`Select ${machine.hostname}`} checked={checked} onCheckedChange={(v) => onChecked(Boolean(v))} />
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
      <button onClick={onDetails} className="shrink-0 text-primary">Details →</button>
    </div>
  </article>;
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
