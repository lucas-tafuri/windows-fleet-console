"use client";

import { useState } from "react";
import { HardDrive, Package, FolderDown, Trash2, Rocket, RefreshCw, Monitor, ArrowRight, LoaderCircle, KeyRound } from "lucide-react";
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

type ActionSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  action: ActionKey | null;
  selectedCount: number;
  busy: boolean;
  lastJob: Job | null;
  software: CatalogApp[];
  mapPrefs: MapPrefs;
  onSubmit: (kind: JobKind, payload: JobPayload) => Promise<void>;
};

export function ActionSheet(props: ActionSheetProps) {
  // Each opening starts a fresh form from saved preferences; live snapshots never overwrite edits.
  return <Sheet open={props.open} onOpenChange={props.onOpenChange}><ActionSheetSession key={`${props.action}:${props.open}`} {...props} /></Sheet>;
}

function ActionSheetSession({ onOpenChange, action, selectedCount, busy, lastJob, software, mapPrefs, onSubmit }: ActionSheetProps) {
  const [pkgId, setPkgId] = useState("");
  const [pkgMode, setPkgMode] = useState<"check" | "install" | "uninstall">(
    "check"
  );
  const [letter, setLetter] = useState(mapPrefs.letter || "Z");
  const [unc, setUnc] = useState(mapPrefs.unc || "");
  const [username, setUsername] = useState(mapPrefs.username || "");
  const [password, setPassword] = useState(mapPrefs.password || "");
  const [target, setTarget] = useState("notepad.exe");
  const [args, setArgs] = useState("");
  const [repo, setRepo] = useState(
    "https://github.com/lucas-tafuri/windows-fleet-console.git"
  );
  const [branch, setBranch] = useState("main");
  const [error, setError] = useState<string | null>(null);

  const [submitted, setSubmitted] = useState(false);
  const ActionIcon = action === "package" ? Package : action === "clean" ? FolderDown : action === "recycle" ? Trash2 : action === "launch" ? Rocket : action === "update" ? RefreshCw : HardDrive;
  const jobKind: JobKind = action === "package" ? pkgMode : action === "map" ? "map_drive" : action === "unmap" ? "unmap_drive" : action === "clean" ? "clean_downloads" : action === "recycle" ? "empty_recycle" : action === "launch" ? "launch" : "self_update";
  const currentJob = submitted && lastJob?.kind === jobKind ? lastJob : null;

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
      setSubmitted(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed");
    }
  }

  return (
      <SheetContent
        side="right"
        className="action-panel w-full gap-0 sm:max-w-lg"
        showCloseButton
      >
        <SheetHeader className="action-panel-header">
          <div className="action-heading-icon"><ActionIcon className="size-5" /></div>
          <p className="fleet-eyebrow">FLEET ACTION</p>
          <SheetTitle className="mt-1 text-2xl font-semibold tracking-tight">{title}</SheetTitle>
          <SheetDescription className="mt-2 text-xs leading-relaxed">
            Configure this action, then run it on your selected machines.
          </SheetDescription>
          <div className="action-target"><Monitor className="size-3.5" />
            <span>{selectedCount} {selectedCount === 1 ? "machine" : "machines"} selected</span>
          </div>
        </SheetHeader>

        <div className="action-panel-body">
          <section className="action-config" aria-label="Action settings">
          <p className="action-section-label">{action === "map" || action === "unmap" ? "Drive settings" : "Configuration"}</p>
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
              <details className="action-credentials" >
                <summary><KeyRound className="size-3.5" /> Share credentials <span>{username || password ? "Account set" : "Optional"}</span></summary>
                <p className="text-xs leading-relaxed text-muted-foreground">Use a specific account to access this network share.</p>
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
                  autoComplete="new-password"
                />
              </div>
              </details>
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
                Saved pairing and automatic startup are preserved.
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

          </section>
          {error ? <p role="alert" className="action-error">{error}</p> : null}
          {currentJob ? <section className="action-results" aria-label="Action results"><p className="action-section-label">Results</p><JobResults job={currentJob} /></section> : null}
        </div>

        <SheetFooter className="action-panel-footer">
          <p className="text-[11px] text-muted-foreground">Applies to {selectedCount} selected {selectedCount === 1 ? "machine" : "machines"}</p>
          <div className="flex gap-3">
            <Button variant="outline" onClick={() => onOpenChange(false)} className="h-10">Close</Button>
            <Button onClick={() => void run()} className="h-10 flex-1" disabled={busy || selectedCount === 0 || !action}>
              {busy ? <LoaderCircle className="size-4 animate-spin" /> : null}
              {busy ? "Sending action…" : action === "package" ? `${pkgMode === "check" ? "Check" : pkgMode === "install" ? "Install" : "Uninstall"} software` : title}
              {!busy ? <ArrowRight className="size-4" /> : null}
            </Button>
          </div>
        </SheetFooter>
      </SheetContent>
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
