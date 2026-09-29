"use client";

import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { JobResults } from "@/components/job-results";
import { useFleet } from "@/hooks/use-fleet";
import { softwareNeedle } from "@/lib/catalog";
import type { Job } from "@/lib/types";
import { SoftwareCell } from "@/components/software-cell";
import { cn } from "@/lib/utils";

export default function SoftwarePage() {
  const {
    data,
    loading,
    error,
    busy,
    submitJob,
    addSoftware,
    removeSoftware,
  } = useFleet();
  const [selected, setSelected] = useState<string[]>([]);
  const [name, setName] = useState("");
  const [wingetId, setWingetId] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [lastJob, setLastJob] = useState<Job | null>(null);

  const liveJob = useMemo(() => {
    if (lastJob) return data.jobs.find((j) => j.id === lastJob.id) || lastJob;
    return null;
  }, [data.jobs, lastJob]);

  const allSelected =
    data.machines.length > 0 &&
    data.machines.every((m) => selected.includes(m.id));

  function toggleAll(next: boolean) {
    setSelected(next ? data.machines.map((m) => m.id) : []);
  }

  function toggleOne(id: string, next: boolean) {
    setSelected((cur) =>
      next ? Array.from(new Set([...cur, id])) : cur.filter((x) => x !== id)
    );
  }

  async function onAdd(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      await addSoftware({
        name: name.trim(),
        match: name.trim(),
        wingetId: wingetId.trim() || undefined,
      });
      setName("");
      setWingetId("");
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Could not add");
    }
  }

  async function checkApp(id: string) {
    const item = data.software.find((s) => s.id === id);
    if (!item || selected.length === 0) return;
    const job = await submitJob("check", selected, {
      package: softwareNeedle(item),
    });
    setLastJob(job);
  }

  async function checkCatalog() {
    if (selected.length === 0) return;
    let job: Job | null = null;
    for (const item of data.software) {
      job = await submitJob("check", selected, {
        package: softwareNeedle(item),
      });
    }
    if (job) setLastJob(job);
  }

  return (
    <div className="flex min-h-full flex-col px-4 py-5 md:px-6">
      <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
        Catalog
      </p>
      <h2 className="mt-1 text-2xl font-medium tracking-tight">Software</h2>
      <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
        Installed versions of the titles you care about, for each client.
        Select clients and check the catalog to refresh their versions.
      </p>

      <details className="mt-5 rounded-xl border border-white/12 bg-card">
        <summary className="cursor-pointer px-4 py-3 text-sm font-medium text-slate-200">Add software to catalog</summary>
      <form
        onSubmit={(e) => void onAdd(e)}
        className="grid gap-3 border-t border-white/10 p-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
      >
        <div className="grid gap-2">
          <Label htmlFor="sw-name">Name</Label>
          <Input
            id="sw-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Adobe After Effects"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="sw-winget">Winget id (optional)</Label>
          <Input
            id="sw-winget"
            className="font-mono"
            value={wingetId}
            onChange={(e) => setWingetId(e.target.value)}
            placeholder="Publisher.Package"
          />
        </div>
        <Button type="submit" disabled={busy || !name.trim()}>
          Add
        </Button>
        {formError ? (
          <p className="text-sm text-destructive sm:col-span-3">{formError}</p>
        ) : null}
      </form>
      </details>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-primary">
          {selected.length} selected
        </span>
        <Button
          disabled={busy || selected.length === 0 || data.software.length === 0}
          onClick={() => void checkCatalog()}
        >
          Check catalog
        </Button>
        <span className="ml-auto text-sm text-slate-300">{data.machines.length} clients · {data.software.length} titles</span>
      </div>

      {loading ? (
        <p className="mt-6 text-sm text-muted-foreground">Loading catalog…</p>
      ) : error ? (
        <p className="mt-6 text-sm text-destructive">{error}</p>
      ) : (
        <div className="mt-4 max-h-[72vh] overflow-auto rounded-xl border border-white/15 bg-card" role="region" aria-label="Software versions by client" tabIndex={0}>
          <table className="w-full border-separate border-spacing-0 text-left text-sm">
            <caption className="sr-only">Installed versions by client. Open Details for installation names and check times.</caption>
            <thead className="text-sm text-slate-100">
              <tr>
                <th scope="col" className="sticky top-0 left-0 z-30 w-12 min-w-12 border-b border-white/15 bg-[#22272e] px-4 py-4">
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={(v) => toggleAll(Boolean(v))}
                    aria-label="Select all machines"
                  />
                </th>
                <th scope="col" className="sticky top-0 left-12 z-30 min-w-32 border-r border-b border-white/15 bg-[#22272e] px-4 py-4 font-semibold">Client</th>
                {data.software.map((item) => (
                  <th scope="col" key={item.id} className="sticky top-0 z-20 min-w-60 border-r border-b border-white/15 bg-[#22272e] px-5 py-4 font-semibold">
                    <div className="flex flex-col gap-1">
                      <span>{item.name}</span>
                      <button
                        type="button"
                        className="w-fit rounded py-1 text-xs font-medium text-primary hover:underline focus-visible:outline-2 focus-visible:outline-primary disabled:text-slate-400"
                        disabled={busy || selected.length === 0}
                        onClick={() => void checkApp(item.id)}
                      >
                        Check
                      </button>
                    </div>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.machines.map((m) => (
                <tr key={m.id} className={cn(selected.includes(m.id) ? "bg-[#28271e]" : "odd:bg-[#191c20] even:bg-[#1e2227]")}>
                  <td className="sticky left-0 z-10 border-b border-white/10 bg-inherit px-4 py-5 align-top">
                    <Checkbox
                      checked={selected.includes(m.id)}
                      onCheckedChange={(v) => toggleOne(m.id, Boolean(v))}
                      aria-label={`Select ${m.hostname}`}
                    />
                  </td>
                  <th scope="row" className="sticky left-12 z-10 border-r border-b border-white/15 bg-inherit px-4 py-5 align-top text-base font-semibold text-white">
                    {m.hostname}
                  </th>
                  {data.software.map((item) => {
                    const cell = data.softwareStatus[m.id]?.[item.id];
                    return (
                      <td key={item.id} className="border-r border-b border-white/10 px-5 py-5 align-top">
                        <SoftwareCell value={cell} title={item.name} />
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
          {data.machines.length === 0 ? (
            <p className="px-4 py-8 text-sm text-muted-foreground">
              No machines yet. Enroll a PC, then check the catalog.
            </p>
          ) : null}
        </div>
      )}

      {data.software.length > 0 ? (
        <ul className="mt-4 flex flex-wrap gap-2">
          {data.software.map((item) => (
            <li
              key={item.id}
              className="flex items-center gap-2 rounded-full border border-white/8 px-3 py-1 text-xs"
            >
              <span>{item.name}</span>
              <button
                type="button"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => void removeSoftware(item.id)}
                disabled={busy}
                aria-label={`Remove ${item.name}`}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      {liveJob ? (
        <div className="mt-6">
          <JobResults job={liveJob} />
        </div>
      ) : null}
    </div>
  );
}
