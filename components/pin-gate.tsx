"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function PinGate({ children }: { children: React.ReactNode }) {
  const [needed, setNeeded] = useState<boolean | null>(null);
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch("/api/fleet", { cache: "no-store" });
        const json = await res.json();
        setNeeded(Boolean(json.pinRequired && !json.unlocked));
      } catch {
        setNeeded(false);
      }
    })();
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const res = await fetch("/api/pin", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ pin }),
    });
    if (!res.ok) {
      setError("That PIN does not match.");
      return;
    }
    setNeeded(false);
  }

  if (needed === null) {
    return (
      <div className="flex min-h-full items-center justify-center text-sm text-muted-foreground">
        Bringing PrettyDamnFleet up…
      </div>
    );
  }

  if (needed) {
    return (
      <div className="flex min-h-full items-center justify-center px-6">
        <form
          onSubmit={submit}
          className="w-full max-w-sm rounded-xl border border-white/8 bg-card p-6 shadow-[0_24px_80px_rgb(0_0_0/0.45)]"
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/pds-logo.png"
            alt=""
            width={130}
            height={237}
            className="h-12 w-auto"
          />
          <p className="mt-3 font-mono text-[11px] tracking-[0.18em] text-primary">
            PrettyDamnFleet
          </p>
          <h1 className="mt-2 text-xl font-medium">Enter dashboard PIN</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            This console can install software and map drives on enrolled PCs.
          </p>
          <div className="mt-5 grid gap-2">
            <Label htmlFor="pin">PIN</Label>
            <Input
              id="pin"
              type="password"
              autoFocus
              value={pin}
              onChange={(e) => setPin(e.target.value)}
            />
          </div>
          {error ? <p className="mt-2 text-sm text-destructive">{error}</p> : null}
          <Button type="submit" className="mt-5 w-full">
            Unlock
          </Button>
        </form>
      </div>
    );
  }

  return children;
}
