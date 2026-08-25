"use client";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useFleet } from "@/hooks/use-fleet";

export function JoinPrompt() {
  const { data, busy, decideJoin } = useFleet();
  const join = (data.pendingJoins || [])[0];

  return (
    <Dialog open={Boolean(join)} onOpenChange={() => undefined}>
      <DialogContent showCloseButton={false} className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Allow this PC to join PrettyDamnFleet?</DialogTitle>
          <DialogDescription>
            A client found this console on the LAN and is waiting. Approve only
            machines you own.
          </DialogDescription>
        </DialogHeader>
        {join ? (
          <dl className="grid gap-2 rounded-lg border border-white/8 bg-black/20 p-3 font-mono text-xs">
            <Row label="Host" value={join.hostname} />
            <Row label="User" value={join.user || "—"} />
            <Row label="OS" value={join.os || "Windows"} />
            <Row label="IP" value={join.ip || "unknown"} />
            {(data.pendingJoins || []).length > 1 ? (
              <Row
                label="Queue"
                value={`${(data.pendingJoins || []).length} waiting`}
              />
            ) : null}
          </dl>
        ) : null}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy || !join}
            onClick={() => join && void decideJoin(join.id, "deny")}
          >
            Deny
          </Button>
          <Button
            disabled={busy || !join}
            onClick={() => join && void decideJoin(join.id, "approve")}
          >
            {busy ? "Saving…" : "Approve"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="truncate text-foreground">{value}</dd>
    </div>
  );
}
