"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

type EnrollInfo = {
  serverUrl: string;
  lanUrls?: string[];
  localhostHint?: boolean;
  command: string;
  pollFallback: string;
  scheduledTask: string;
  repo?: string;
  oneLiner?: string;
};

export default function EnrollPage() {
  const [info, setInfo] = useState<EnrollInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch("/api/enroll", { cache: "no-store" });
        if (!res.ok) throw new Error("Could not load enroll info");
        setInfo((await res.json()) as EnrollInfo);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed");
      }
    })();
  }, []);

  async function copy(label: string, value: string) {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    setTimeout(() => setCopied(null), 1500);
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col px-4 py-5 md:px-6">
      <p className="font-mono text-[11px] tracking-[0.22em] text-muted-foreground uppercase">
        Agents
      </p>
      <h2 className="mt-1 text-2xl font-medium tracking-tight">Enroll a PC</h2>
      <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
        Run the installer on a PC you own. It finds this console on the LAN,
        then waits until you Approve it here. Keep this dashboard open.
      </p>

      {error ? <p className="mt-4 text-sm text-destructive">{error}</p> : null}

      <ol className="mt-8 grid gap-6">
        <Step n="00" title="Keep the console reachable">
          <p>
            The installer broadcasts on UDP{" "}
            <span className="font-mono">43124</span> and checks HTTP{" "}
            <span className="font-mono">43123</span>. If Windows Firewall
            prompts, allow PrettyDamnFleet / Node on the private network.
          </p>
          {info?.lanUrls && info.lanUrls.length > 0 ? (
            <p className="mt-2 font-mono text-[11px] text-foreground/80">
              This host: {info.lanUrls.join("  ·  ")}
            </p>
          ) : null}
        </Step>
        <Step n="01" title="Run the installer on the PC">
          From PowerShell. No URL or token is required:
          {info?.oneLiner ? (
            <CopyBlock
              value={info.oneLiner}
              copied={copied === "one"}
              onCopy={() => copy("one", info.oneLiner!)}
            />
          ) : (
            <p>Loading command…</p>
          )}
          If you already copied{" "}
          <code className="font-mono text-foreground/90">dist\\install.cmd</code>{" "}
          next to the exe, double-click it:
          {info ? (
            <CopyBlock
              value={info.command}
              copied={copied === "run"}
              onCopy={() => copy("run", info.command)}
            />
          ) : null}
        </Step>
        <Step n="02" title="Approve the PC on this screen">
          When the installer finds this console, a dialog asks to allow the
          hostname, user, and IP. Deny unknown machines. After Approve, the
          agent installs and starts at Windows logon.
          {info ? (
            <p className="mt-2 font-mono text-[11px] text-foreground/80">
              {info.scheduledTask}
            </p>
          ) : null}
        </Step>
        <Step n="03" title="HTTP-only networks">
          If a proxy eats WebSockets, add{" "}
          <span className="font-mono">-HttpOnly</span>:
          {info ? (
            <CopyBlock
              value={info.pollFallback}
              copied={copied === "poll"}
              onCopy={() => copy("poll", info.pollFallback)}
            />
          ) : null}
        </Step>
      </ol>
    </div>
  );
}

function Step({
  n,
  title,
  children,
}: {
  n: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <li className="rounded-xl border border-white/8 bg-card/50 p-4">
      <p className="font-mono text-[11px] text-primary">{n}</p>
      <h3 className="mt-1 text-base font-medium">{title}</h3>
      <div className="mt-2 text-sm text-muted-foreground">{children}</div>
    </li>
  );
}

function CopyBlock({
  value,
  copied,
  onCopy,
}: {
  value: string;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <div className="mt-3 rounded-lg border border-white/8 bg-black/30 p-3">
      <pre className="overflow-x-auto font-mono text-[11px] leading-relaxed text-foreground/85 whitespace-pre-wrap">
        {value}
      </pre>
      <Button variant="outline" size="sm" className="mt-2" onClick={onCopy}>
        {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  );
}
