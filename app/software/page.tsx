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
        Titles you care about across the fleet. Check uses the uninstall
        registry (and winget when the id is known).
      </p>

      <form
        onSubmit={(e) => void onAdd(e)}
        className="mt-5 grid gap-3 rounded-xl border border-white/8 bg-card/70 p-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
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

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <span className="font-mono text-[11px] text-primary">
          {selected.length} selected
        </span>
        <Button
          variant="outline"
          disabled={busy || selected.length === 0 || data.software.length === 0}
          onClick={() => void checkCatalog()}
        >
          Check catalog
        </Button>
      </div>

      {loading ? (
        <p className="mt-6 text-sm text-muted-foreground">Loading catalog…</p>
      ) : error ? (
        <p className="mt-6 text-sm text-destructive">{error}</p>
      ) : (
        <div className="mt-4 overflow-x-auto rounded-xl border border-white/8 bg-card/80">
          <table className="w-full min-w-[640px] text-left text-sm">
            <thead className="border-b border-white/8 text-[11px] tracking-wide text-muted-foreground uppercase">
              <tr>
                <th className="w-10 px-3 py-2.5">
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={(v) => toggleAll(Boolean(v))}
                    aria-label="Select all machines"
                  />
                </th>
                <th className="px-2 py-2.5 font-medium">Host</th>
                {data.software.map((item) => (
                  <th key={item.id} className="px-2 py-2.5 font-medium">
                    <div className="flex flex-col gap-1">
                      <span>{item.name}</span>
                      <button
                        type="button"
                        className="w-fit text-[10px] tracking-normal text-primary normal-case hover:underline disabled:text-muted-foreground"
                        disabled={busy || selected.length === 0}
                        onClick={() => void checkApp(item.id)}
                      >
                        Check
                      </button>
                    </div>
                  </th>
                ))}
                <th className="px-2 py-2.5 font-medium"> </th>
              </tr>
            </thead>
            <tbody>
              {data.machines.map((m) => (
                <tr key={m.id} className="border-b border-white/6 last:border-0">
                  <td className="px-3 py-2.5">
                    <Checkbox
                      checked={selected.includes(m.id)}
                      onCheckedChange={(v) => toggleOne(m.id, Boolean(v))}
                      aria-label={`Select ${m.hostname}`}
                    />
                  </td>
                  <td className="px-2 py-2.5 font-mono text-[13px]">
                    {m.hostname}
                  </td>
                  {data.software.map((item) => {
                    const cell = data.softwareStatus[m.id]?.[item.id];
                    return (
                      <td key={item.id} className="px-2 py-2.5">
                        <StatusPill installed={cell?.installed} known={Boolean(cell)} />
                      </td>
                    );
                  })}
                  <td />
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

function StatusPill({
  installed,
  known,
}: {
  installed?: boolean;
  known: boolean;
}) {
  if (!known) {
    return <span className="text-xs text-muted-foreground">Unknown</span>;
  }
  return (
    <span
      className={cn(
        "rounded-full px-2 py-0.5 text-[11px]",
        installed
          ? "bg-primary/15 text-primary"
          : "bg-white/6 text-muted-foreground"
      )}
    >
      {installed ? "Installed" : "Missing"}
    </span>
  );
}
