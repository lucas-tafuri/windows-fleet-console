"use client";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { fleetRequest, useFleet } from "@/hooks/use-fleet";

type Enrollment = { fleetId: string; nodeId: string; peerUrl: string; dashboardUrl: string };
export default function EnrollPage() {
  const { data } = useFleet();
  const [info, setInfo] = useState<Enrollment | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { void fetch("/api/v2/enroll").then((r) => r.json()).then(setInfo).catch(() => setError("Cannot reach this node")); }, []);
  const command = info ? `.\\install-mesh.ps1 -Mode Join -Peer '${info.peerUrl}' -Fingerprint '${info.nodeId}' -Fleet '${info.fleetId}'` : "";
  return <main className="mx-auto w-full max-w-4xl px-4 py-6 md:px-6">
    <p className="fleet-eyebrow">FLEET CONNECTIONS</p><h1 className="mt-2 text-2xl font-medium">Every PC is a manager</h1>
    <p className="mt-3 text-sm text-muted-foreground">Install the same release on each PC. Choose Join fleet, then approve its fingerprint here. The first PC has no special role once peers have synchronized.</p>
    {error ? <p role="alert" className="mt-4 text-destructive">{error}</p> : null}
    {info ? <section className="mt-6 space-y-3 rounded-xl border border-white/10 p-4"><h2 className="font-medium">Join through this PC</h2><p className="text-sm text-muted-foreground">Run the release installer as administrator with these details. Keep this dashboard open to approve the new PC.</p><pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-black/20 p-3 text-xs select-all">{command}</pre><p className="text-xs text-muted-foreground">Fleet fingerprint</p><p className="break-all font-mono text-xs select-all">{info.fleetId}</p></section> : null}
    <section className="mt-8"><h2 className="text-lg font-medium">Peer dashboards</h2><p className="mt-1 text-sm text-muted-foreground">A disconnected peer keeps its own history. Its records appear here after reconnection.</p><ul className="mt-4 divide-y divide-white/10">{(data.peers || []).map((p) => <li key={p.member.id} className="flex flex-wrap items-center justify-between gap-3 py-4"><div><a className="text-primary" href={p.member.dashboardUrl}>{p.member.hostname}</a><p className="mt-1 text-xs text-muted-foreground">{data.serverTime - p.lastSeen < 20_000 ? "Reachable" : "Unreachable from this PC"} · {p.lastSync ? `Last synchronized ${new Date(p.lastSync).toLocaleString()}` : "Awaiting synchronization"}</p>{p.error ? <p className="mt-1 text-xs text-muted-foreground">{p.error}</p> : null}</div><Button variant="outline" onClick={() => { if (window.confirm(`Remove ${p.member.hostname} from this fleet? This identity cannot rejoin; it will need a new enrollment.`)) void fleetRequest(`/remove/${p.member.id}`, {}).catch((e: Error) => setError(e.message)); }}>Remove</Button></li>)}</ul></section>
  </main>;
}
