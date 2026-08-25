"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { JobResults } from "@/components/job-results";
import { softwareNeedle } from "@/lib/catalog";
import type { CatalogApp, Job, JobKind, JobPayload, MapPrefs } from "@/lib/types";

export type ActionKey =
  | "package"
  | "map"
  | "unmap"
  | "clean"
  | "recycle"
  | "launch"
  | "update";

const LETTERS = "DEFGHIJKLMNOPQRSTUVWXYZ".split("");

export function ActionSheet({
  open,
  onOpenChange,
  action,
  selectedCount,
  busy,
  lastJob,
  software,
  mapPrefs,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  action: ActionKey | null;
  selectedCount: number;
  busy: boolean;
  lastJob: Job | null;
  software: CatalogApp[];
  mapPrefs: MapPrefs;
  onSubmit: (kind: JobKind, payload: JobPayload) => Promise<void>;
}) {
  const [pkgId, setPkgId] = useState("");
  const [pkgMode, setPkgMode] = useState<"check" | "install" | "uninstall">(
    "check"
  );
  const [letter, setLetter] = useState("Z");
  const [unc, setUnc] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [target, setTarget] = useState("notepad.exe");
  const [args, setArgs] = useState("");
  const [repo, setRepo] = useState(
    "https://github.com/lucas-tafuri/windows-fleet-console.git"
  );
  const [branch, setBranch] = useState("main");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || (action !== "map" && action !== "unmap")) return;
    if (mapPrefs.letter) setLetter(mapPrefs.letter);
    if (action === "map") {
      if (mapPrefs.unc) setUnc(mapPrefs.unc);
      setUsername(mapPrefs.username || "");
      setPassword(mapPrefs.password || "");
    }
    // Prefill when the sheet opens; do not clobber in-progress edits on live snapshots.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, action]);

  const title =
    action === "package"
      ? "Software"
      : action === "map"
        ? "Map drive"
        : action === "unmap"
          ? "Unmap drive"
          : action === "clean"
            ? "Clean Downloads"
            : action === "recycle"
              ? "Empty Recycle Bin"
              : action === "launch"
                ? "Launch program"
                : action === "update"
                  ? "Update & restart"
                  : "Action";

  async function run() {
    setError(null);
    try {
      if (action === "package") {
        const item = software.find((s) => s.id === pkgId) || software[0];
        if (!item) {
          setError("Add titles on the Software page first.");
          return;
        }
        await onSubmit(pkgMode, { package: softwareNeedle(item) });
      } else if (action === "map") {
        await onSubmit("map_drive", {
          letter,
          unc: unc.trim(),
          username: username.trim() || undefined,
          password: password || undefined,
        });
      } else if (action === "unmap") {
        await onSubmit("unmap_drive", { letter });
      } else if (action === "clean") {
        await onSubmit("clean_downloads", {});
      } else if (action === "recycle") {
        await onSubmit("empty_recycle", {});
      } else if (action === "launch") {
        await onSubmit("launch", {
          target: target.trim(),
          args: args.trim() || undefined,
        });
      } else if (action === "update") {
        await onSubmit("self_update", {
          repo: repo.trim(),
          branch: branch.trim() || "main",
        });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed");
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full gap-0 sm:max-w-md"
        showCloseButton
      >
        <SheetHeader className="border-b border-white/8">
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            Runs on {selectedCount} selected{" "}
            {selectedCount === 1 ? "machine" : "machines"}. The fleet stays
            visible behind this panel.
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
          {action === "package" ? (
            <>
              <div className="flex rounded-lg border border-white/8 p-0.5">
                {(["check", "install", "uninstall"] as const).map((mode) => (
                  <button
                    key={mode}
                    type="button"
                    onClick={() => setPkgMode(mode)}
                    className={`flex-1 rounded-md py-1.5 text-xs capitalize transition-colors duration-150 ${
                      pkgMode === mode
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    {mode === "check" ? "Check" : mode}
                  </button>
                ))}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="pkg">Catalog title</Label>
                <select
                  id="pkg"
                  value={pkgId || software[0]?.id || ""}
                  onChange={(e) => setPkgId(e.target.value)}
                  className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm"
                >
                  {software.map((item) => (
                    <option key={item.id} value={item.id} className="bg-card">
                      {item.name}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-muted-foreground">
                  Managed on the Software page. Check uses display-name
                  matching; install/uninstall need a winget id when the title
                  is not in winget.
                </p>
              </div>
            </>
          ) : null}

          {action === "map" ? (
            <>
              <LetterSelect value={letter} onChange={setLetter} />
              <div className="grid gap-2">
                <Label htmlFor="unc">UNC path</Label>
                <Input
                  id="unc"
                  className="font-mono"
                  value={unc}
                  onChange={(e) => setUnc(e.target.value)}
                  list="unc-history"
                  placeholder="\\server\share"
                />
                <datalist id="unc-history">
                  {(mapPrefs.uncHistory || []).map((path) => (
                    <option key={path} value={path} />
                  ))}
                </datalist>
              </div>
              <p className="text-xs text-muted-foreground">
                Machines that already have this letter or share mapped are
                skipped.
              </p>
              <div className="grid gap-2">
                <Label htmlFor="user">Username (optional)</Label>
                <Input
                  id="user"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="pass">Password (optional)</Label>
                <Input
                  id="pass"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
            </>
          ) : null}

          {action === "unmap" ? (
            <LetterSelect value={letter} onChange={setLetter} />
          ) : null}

          {action === "clean" ? (
            <p className="text-sm text-muted-foreground">
              Deletes files in the signed-in user&apos;s Downloads folder.
              Locked files are skipped and counted.
            </p>
          ) : null}

          {action === "recycle" ? (
            <p className="text-sm text-muted-foreground">
              Empties the Recycle Bin for the signed-in user. Confirmation
              dialogs are suppressed on the PC.
            </p>
          ) : null}

          {action === "launch" ? (
            <>
              <div className="grid gap-2">
                <Label htmlFor="target">Program</Label>
                <Input
                  id="target"
                  className="font-mono"
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder="notepad.exe"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="args">Arguments (optional)</Label>
                <Input
                  id="args"
                  className="font-mono"
                  value={args}
                  onChange={(e) => setArgs(e.target.value)}
                />
              </div>
            </>
          ) : null}

          {action === "update" ? (
            <>
              <p className="text-sm text-muted-foreground">
                Each selected PC runs <span className="font-mono">git pull</span>{" "}
                in its fleet checkout, replaces the agent, and restarts. If Git
                is missing it clones or downloads{" "}
                <span className="font-mono">fleet-agent.exe</span> from GitHub.
                First run already registers the agent at Windows logon.
              </p>
              <div className="grid gap-2">
                <Label htmlFor="repo">Git remote</Label>
                <Input
                  id="repo"
                  className="font-mono"
                  value={repo}
                  onChange={(e) => setRepo(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="branch">Branch</Label>
                <Input
                  id="branch"
                  className="font-mono"
                  value={branch}
                  onChange={(e) => setBranch(e.target.value)}
                />
              </div>
            </>
          ) : null}

          {error ? <p className="text-sm text-destructive">{error}</p> : null}
          {lastJob ? <JobResults job={lastJob} /> : null}
        </div>

        <SheetFooter className="border-t border-white/8">
          <Button onClick={() => void run()} disabled={busy || selectedCount === 0}>
            {busy ? "Sending…" : "Run on selection"}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function LetterSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor="letter">Drive letter</Label>
      <select
        id="letter"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 rounded-lg border border-input bg-transparent px-2.5 font-mono text-sm"
      >
        {LETTERS.map((l) => (
          <option key={l} value={l} className="bg-card">
            {l}:
          </option>
        ))}
      </select>
    </div>
  );
}
