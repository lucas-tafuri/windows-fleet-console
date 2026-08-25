import { cn } from "@/lib/utils";
import type { MachineStatus } from "@/lib/types";

const COLOR: Record<MachineStatus, string> = {
  online: "bg-online",
  under_load: "bg-load",
  frozen: "bg-frozen",
  offline: "bg-offline",
  limited: "bg-limited",
};

export function StatusLamp({
  status,
  className,
}: {
  status: MachineStatus;
  className?: string;
}) {
  const live = status !== "offline";
  return (
    <span
      className={cn(
        "inline-flex size-2 rounded-full",
        COLOR[status],
        live && "lamp-live",
        className
      )}
      aria-hidden
    />
  );
}
