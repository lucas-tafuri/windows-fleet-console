"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";

export const REPORTS = [
  { id: "system", name: "System overview", description: "Hardware, Windows version, memory and uptime.", icon: "◈" },
  { id: "disks", name: "Disk space", description: "Local volumes, free space and low-space warnings.", icon: "▤" },
  { id: "network", name: "Network configuration", description: "Adapters, addresses, gateways and DNS servers.", icon: "⇄" },
  { id: "processes", name: "Top processes", description: "40 largest processes by memory. CPU shows lifetime seconds.", icon: "▥" },
  { id: "services", name: "Windows services", description: "Up to 100 running or automatic services, stopped first.", icon: "⚙" },
  { id: "events", name: "Recent Windows errors", description: "Latest 20 critical and error events from the last 24 hours.", icon: "!" },
] as const;

export function downloadReport(name: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url; link.download = name.replace(/[^a-z0-9._-]/gi, "_"); link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function DiagnosticReport({ output, hostname }: { output: string; hostname: string }) {
  const [search, setSearch] = useState("");
  let report: { profile: string; capturedAt: string; rows: Record<string, unknown>[] };
  try {
    report = JSON.parse(output);
    if (!Array.isArray(report.rows) || !report.rows.every((r) => r && typeof r === "object" && !Array.isArray(r))) throw new Error();
  } catch { return <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs">{output}</pre>; }
  const rows = report.rows.filter((row) => JSON.stringify(row).toLowerCase().includes(search.toLowerCase()));
  const columns = Array.from(new Set(report.rows.flatMap(Object.keys)));
  const cell = (value: unknown) => value == null ? "—" : typeof value === "object" ? JSON.stringify(value) : String(value);
  return <div className="mt-3 space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">Snapshot · {new Date(report.capturedAt).toLocaleString()}</span>
      <Button variant="outline" size="sm" onClick={() => downloadReport(`${hostname}-${report.profile}.json`, output)}>Download report</Button>
    </div>
    {report.rows.length > 1 && <input className="tool-input" aria-label={`Search ${hostname} report`} placeholder="Filter this report…" value={search} onChange={(e) => setSearch(e.target.value)} />}
    {report.rows.length === 0 ? <p className="text-sm text-muted-foreground">No matching entries found on this PC.</p> : report.profile === "system" ? <dl className="machine-facts">{Object.entries(report.rows[0]).map(([key, value]) => <div key={key}><dt>{key.replace(/([a-z])([A-Z])/g, "$1 $2")}</dt><dd>{cell(value)}</dd></div>)}</dl> : <div className="report-scroll"><table className="fleet-table"><thead><tr>{columns.map((key) => <th key={key}>{key.replace(/([a-z])([A-Z])/g, "$1 $2")}</th>)}</tr></thead><tbody>{rows.map((row, i) => <tr key={i} className={row.Attention === "Low disk space" ? "text-load" : ""}>{columns.map((key) => <td key={key}>{cell(row[key])}</td>)}</tr>)}</tbody></table>{rows.length === 0 && <p className="p-4 text-sm">No entries match this filter.</p>}</div>}
  </div>;
}
