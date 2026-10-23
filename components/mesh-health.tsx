"use client";
import { useState } from "react";
import { fleetRequest, useFleet } from "@/hooks/use-fleet";

export function MeshHealth() {
  const { data, error } = useFleet();
  const [message, setMessage] = useState("");
  const reachable = (data.peers || []).filter((p) => data.serverTime - p.lastSeen < 20_000).length;
  let links = (data.peers || []).map((p) => ({ name: p.member.hostname, url: p.member.dashboardUrl }));
  if (!links.length) { try { links = JSON.parse(localStorage.getItem("fleet-peer-links") || "[]"); } catch { /* optional */ } }
  return <div className="border-b border-white/8 px-4 py-2 text-xs text-muted-foreground" aria-live="polite">
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
      <span>{error || (data.removed ? "This node has been removed from the fleet" : reachable ? `${reachable} reachable ${reachable === 1 ? "peer" : "peers"} · local dashboard available` : "Working locally · new records have no confirmed backup until a peer connects")}</span>
      <a className="text-primary" href="/enroll">Fleet connections</a>
    </div>
    {error ? <div className="mt-2 flex flex-wrap gap-4">{links.filter((p) => { try { const u = new URL(p.url); return u.protocol === "http:" && !u.username && !u.password; } catch { return false; } }).map((p) => <a className="text-primary underline" key={p.url} href={p.url}>{p.name}</a>)}</div> : null}
    {data.clockUncertain ? <div className="mt-2 text-amber-300">Queued work is held because the clocks disagree. Correct Windows time on affected PCs. <button className="underline" onClick={() => void fleetRequest("/clock/acknowledge", {}).catch((e: Error) => setMessage(e.message))}>I corrected this PC’s clock</button></div> : null}
    {message ? <p role="alert">{message}</p> : null}
    {data.duplicateIdentity ? <p role="alert" className="mt-2 text-destructive">A second installation is using this node’s identity. Execution is stopped. Re-enroll the copied installation with a fresh identity.</p> : null}
  </div>;
}
