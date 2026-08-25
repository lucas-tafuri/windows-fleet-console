"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

type EnrollInfo = {
  token: string;
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
        Run the install script on a PC you own. It installs Git if needed,
        downloads the agent, registers it at Windows logon, and starts it. The
        agent phones home — no inbound ports on the PC.
      </p>

      {error ? <p className="mt-4 text-sm text-destructive">{error}</p> : null}

      <ol className="mt-8 grid gap-6">
        <Step n="00" title="Console URL for other PCs">
          <p>
            Agents cannot use{" "}
            <span className="font-mono">http://127.0.0.1:43123</span> unless
            the console is running on that same PC. Use the address of the
            machine that hosts this dashboard, port{" "}
            <span className="font-mono">43123</span>.
          </p>
          {info ? (
            <div className="mt-3 grid gap-2">
              <p className="text-xs tracking-wide text-muted-foreground uppercase">
                Use this as -Server
              </p>
              <CopyBlock
                value={info.serverUrl}
                copied={copied === "url"}
                onCopy={() => copy("url", info.serverUrl)}
              />
              {info.lanUrls && info.lanUrls.length > 0 ? (
                <p className="font-mono text-[11px] text-foreground/80">
                  Detected on this host: {info.lanUrls.join("  ·  ")}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  On the console host, run{" "}
                  <span className="font-mono">ipconfig</span> (Windows) or{" "}
                  <span className="font-mono">hostname -I</span> (Linux) and
                  use <span className="font-mono">http://THAT_IP:43123</span>.
                </p>
              )}
              {info.localhostHint &&
              (!info.lanUrls || info.lanUrls.length === 0) ? (
                <p className="text-xs text-load">
                  This page was opened via localhost and no LAN IP was
                  detected. Other machines will not reach 127.0.0.1.
                </p>
              ) : null}
            </div>
          ) : null}
        </Step>
        <Step n="01" title="Run the install script on the PC">
          From PowerShell. The window stays open and a log is written to{" "}
          <span className="font-mono text-foreground/80">
            %TEMP%\fleet-console-install.log
          </span>
          :
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
          next to the exe, double-click it or:
          {info ? (
            <CopyBlock
              value={info.command}
              copied={copied === "run"}
              onCopy={() => copy("run", info.command)}
            />
          ) : null}
        </Step>
        <Step n="02" title="Startup is automatic">
          The script (and the first agent launch) copies the exe to{" "}
          <span className="font-mono text-foreground/80">
            %LOCALAPPDATA%\FleetConsole
          </span>{" "}
          and registers a logon scheduled task, with a Startup-folder fallback.
          {info ? (
            <p className="mt-2 font-mono text-[11px] text-foreground/80">
              {info.scheduledTask}
            </p>
          ) : null}
        </Step>
        <Step n="03" title="HTTP-only networks">
          If a proxy eats WebSockets:
          {info ? (
            <CopyBlock
              value={info.pollFallback}
              copied={copied === "poll"}
              onCopy={() => copy("poll", info.pollFallback)}
            />
          ) : null}
        </Step>
      </ol>

      {info ? (
        <dl className="mt-8 grid gap-2 rounded-xl border border-white/8 bg-card/60 p-4 font-mono text-xs">
          <div className="flex justify-between gap-4">
            <dt className="text-muted-foreground">Server</dt>
            <dd>{info.serverUrl}</dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt className="text-muted-foreground">Token</dt>
            <dd className="truncate">{info.token}</dd>
          </div>
        </dl>
      ) : null}
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
