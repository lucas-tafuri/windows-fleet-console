"use client";

import { useMemo, useState } from "react";
import {
  FolderDown,
  HardDrive,
  Package,
  RefreshCw,
  Rocket,
  Trash2,
  Unplug,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
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
  const [filter, setFilter] = useState<"all" | MachineStatus>("all");
  const [selected, setSelected] = useState<string[]>([]);
  const [action, setAction] = useState<ActionKey | null>(null);
  const [lastJob, setLastJob] = useState<Job | null>(null);

  const filtered = useMemo(() => {
    if (filter === "all") return data.machines;
    return data.machines.filter((m) => m.status === filter);
  }, [data.machines, filter]);

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
    if (next) setSelected(filtered.map((m) => m.id));
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

        <header className="flex flex-wrap items-end justify-between gap-3 px-4 pt-5 pb-3 md:px-6">
          <div>
            <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
              Machines · {transport === "ws" ? "live socket" : "http poll"}
            </p>
            <h2 className="mt-1 text-2xl font-medium tracking-tight">Fleet</h2>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {FILTERS.map((f) => {
              const n = f.id === "all" ? counts.all : counts[f.id] || 0;
              const on = filter === f.id;
              return (
                <button
                  key={f.id}
                  type="button"
                  onClick={() => setFilter(f.id)}
                  className={cn(
                    "rounded-full border px-2.5 py-1 text-[11px] transition-colors duration-150",
                    on
                      ? "border-primary/40 bg-accent text-primary"
                      : "border-white/8 text-muted-foreground hover:text-foreground"
                  )}
                >
                  {f.label}
                  <span className="tabular ml-1.5 font-mono">{n}</span>
                </button>
              );
            })}
          </div>
        </header>

        <div className="min-h-0 flex-1 overflow-auto px-4 pb-8 md:px-6">
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
                : "Try another status, or select All."}
            </EmptyState>
          ) : (
            <>
              <div className="hidden overflow-hidden rounded-xl border border-white/8 bg-card/80 md:block">
                <table className="w-full text-left text-sm">
                  <thead className="border-b border-white/8 text-[11px] tracking-wide text-muted-foreground uppercase">
                    <tr>
                      <th className="w-10 px-3 py-2.5">
                        <Checkbox
                          checked={allFilteredSelected}
                          onCheckedChange={(v) => toggleAll(Boolean(v))}
                          aria-label="Select all"
                        />
                      </th>
                      <th className="px-2 py-2.5 font-medium">Host</th>
                      <th className="px-2 py-2.5 font-medium">User</th>
                      <th className="px-2 py-2.5 font-medium">Status</th>
                      <th className="px-2 py-2.5 font-medium">CPU</th>
                      <th className="px-2 py-2.5 font-medium">Memory</th>
                      <th className="px-2 py-2.5 font-medium">Seen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((m) => (
                      <MachineRow
                        key={m.id}
                        machine={m}
                        checked={selected.includes(m.id)}
                        onChecked={(v) => toggleOne(m.id, v)}
                      />
                    ))}
                  </tbody>
                </table>
              </div>

              <div className="grid gap-2 md:hidden">
                {filtered.map((m) => (
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => toggleOne(m.id, !selected.includes(m.id))}
                    className={cn(
                      "rounded-xl border bg-card/80 p-3 text-left transition-colors duration-150",
                      selected.includes(m.id)
                        ? "border-primary/50 bg-accent/40"
                        : "border-white/8"
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="flex items-center gap-2 font-mono text-sm">
                        <StatusLamp status={m.status} />
                        {m.hostname}
                      </span>
                      <Badge variant="outline">{STATUS_LABEL[m.status]}</Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {m.user || "no interactive user"} · {m.lastSeenLabel}
                    </p>
                    <div className="mt-3 flex gap-4">
                      <Meter label="CPU" value={m.cpu} />
                      <Meter label="RAM" value={m.memory} />
                    </div>
                  </button>
                ))}
              </div>
            </>
          )}
        </div>

        <div
          className={cn(
            "fixed right-0 bottom-16 left-0 z-20 border-t border-white/8 bg-[#161410]/95 px-3 py-2 backdrop-blur-md transition-[opacity,transform] duration-200 ease-out md:bottom-0 md:left-52 md:px-6 lg:right-80",
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

      <aside className="hidden w-80 shrink-0 border-l border-white/8 bg-card/40 lg:flex lg:flex-col">
        <div className="border-b border-white/8 px-4 py-4">
          <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
            Live rail
          </p>
          <h3 className="mt-1 text-sm font-medium">Latest job</h3>
        </div>
        <div className="flex-1 overflow-auto p-4">
          {liveJob ? (
            <JobResults job={liveJob} />
          ) : (
            <p className="text-sm text-muted-foreground">
              Select machines and run an action. Per-PC results land here,
              including which fallback ran.
            </p>
          )}
        </div>
      </aside>

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

function MachineRow({
  machine,
  checked,
  onChecked,
}: {
  machine: MachineView;
  checked: boolean;
  onChecked: (next: boolean) => void;
}) {
  return (
    <tr
      className={cn(
        "border-b border-white/6 transition-colors duration-150 last:border-0",
        checked ? "bg-accent/35" : "hover:bg-white/3"
      )}
    >
      <td className="px-3 py-3">
        <Checkbox
          checked={checked}
          onCheckedChange={(v) => onChecked(Boolean(v))}
          aria-label={`Select ${machine.hostname}`}
        />
      </td>
      <td className="px-2 py-3">
        <div className="flex items-center gap-2">
          <StatusLamp status={machine.status} />
          <span className="font-mono text-[13px]">{machine.hostname}</span>
          {machine.demo ? (
            <span className="font-mono text-[10px] text-primary/80">demo</span>
          ) : (
            <span className="font-mono text-[10px] text-muted-foreground">
              {machine.transport}
            </span>
          )}
        </div>
        {machine.mappedDrives.length > 0 ? (
          <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
            {machine.mappedDrives
              .map((d) => `${d.letter}: ${d.path}`)
              .join(" · ")}
          </p>
        ) : null}
      </td>
      <td className="px-2 py-3 text-xs text-muted-foreground">
        {machine.user || "—"}
      </td>
      <td className="px-2 py-3">
        <span
          className={cn(
            "text-xs",
            machine.status === "online" && "text-online",
            machine.status === "under_load" && "text-load",
            machine.status === "frozen" && "text-frozen",
            machine.status === "offline" && "text-offline",
            machine.status === "limited" && "text-limited"
          )}
        >
          {STATUS_LABEL[machine.status]}
        </span>
      </td>
      <td className="px-2 py-3">
        <Meter label="CPU" value={machine.cpu} />
      </td>
      <td className="px-2 py-3">
        <Meter label="RAM" value={machine.memory} />
      </td>
      <td className="tabular px-2 py-3 font-mono text-[11px] text-muted-foreground">
        {machine.lastSeenLabel}
      </td>
    </tr>
  );
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
