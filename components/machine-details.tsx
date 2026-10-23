"use client";

import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { Meter } from "@/components/meter";
import { STATUS_LABEL } from "@/lib/status";
import { JOB_LABEL } from "@/lib/labels";
import type { FleetSnapshot, MachineView } from "@/lib/types";

export function MachineDetails({ machine, data, onClose }: { machine: MachineView | undefined; data: FleetSnapshot; onClose: () => void }) {
  const peer = data.peers?.find((p) => p.member.id === machine?.id);
  const local = machine?.id === data.nodeId;
  const recent = data.jobs.filter((j) => j.machineIds.includes(machine?.id || "")).slice(0, 5);
  const offline = machine?.status === "offline";
  return <Sheet open={Boolean(machine)} onOpenChange={(open) => { if (!open) onClose(); }}><SheetContent className="!w-full sm:!max-w-xl overflow-y-auto">
    {machine && <><SheetHeader className="px-6 pt-7"><p className="fleet-eyebrow">MACHINE DETAILS</p><SheetTitle className="mt-2 text-2xl">{machine.hostname}</SheetTitle><SheetDescription>{local ? "You are using this node’s dashboard." : (machine.transport === "mesh" && !machine.historical) ? "An independent member of your fleet." : "Historical machine · not enrolled in the mesh."}</SheetDescription></SheetHeader>
    <div className="px-6 pb-6 space-y-6"><div className="rounded-xl border border-white/10 bg-white/3 p-4"><div className="flex justify-between text-sm"><strong>{STATUS_LABEL[machine.status]}</strong><span className="text-muted-foreground">{machine.lastSeenLabel}</span></div>{offline && <p className="mt-2 text-xs text-load">Saved observations may be stale. This node cannot currently reach this PC.</p>}<div className="mt-4 grid grid-cols-3 gap-4"><Meter label="CPU" value={offline ? null : machine.cpu} /><Meter label="RAM" value={offline ? null : machine.memory} /><Meter label="GPU" value={offline ? null : machine.gpu} /></div></div>
    <dl className="machine-facts"><div><dt>Signed-in user</dt><dd>{machine.user || "None reported"}</dd></div><div><dt>Operating system</dt><dd>{machine.os}</dd></div><div><dt>Dashboard</dt><dd>{machine.dashboardUrl ? <a className="text-primary underline" href={machine.dashboardUrl}>Open {machine.hostname} ↗</a> : "Not available"}</dd></div><div><dt>Last synchronization</dt><dd>{local ? "Local records" : peer?.lastSync ? new Date(peer.lastSync).toLocaleString() : "Not yet confirmed"}</dd></div>{peer?.error && <div><dt>Connection issue</dt><dd className="text-load">{peer.error}</dd></div>}<div><dt>Node identity</dt><dd className="break-all font-mono text-[10px]">{machine.id}</dd></div></dl>
    {(machine.transport === "mesh" && !machine.historical) && <a className="flex items-center justify-center rounded-lg bg-primary px-4 py-3 font-medium text-primary-foreground" href={`/tools?machine=${encodeURIComponent(machine.id)}`}>Open Windows tools →</a>}
    <section><h3 className="section-heading">Mapped drives</h3>{machine.mappedDrives.length ? <ul className="space-y-2">{machine.mappedDrives.map((d) => <li className="rounded-lg bg-white/3 p-3 text-xs break-all" key={d.letter}><strong className="text-primary mr-3">{d.letter}</strong>{d.path}</li>)}</ul> : <p className="text-sm text-muted-foreground">No mapped drives reported for the active session.</p>}</section>
    <section><h3 className="section-heading">Recent activity</h3>{recent.length ? recent.map((j) => <a href={`/jobs?machine=${encodeURIComponent(machine.id)}`} className="block border-b border-white/8 py-3" key={j.id}><div className="flex justify-between gap-2 text-sm"><span>{JOB_LABEL[j.kind]}{j.payload.profile ? ` · ${j.payload.profile}` : ""}</span><span className="text-primary">{j.results[machine.id]?.status || j.status}</span></div><p className="mt-1 text-xs text-muted-foreground">{new Date(j.createdAt).toLocaleString()}</p></a>) : <p className="text-sm text-muted-foreground">No recorded jobs for this machine.</p>}</section></div></>}
  </SheetContent></Sheet>;
}
