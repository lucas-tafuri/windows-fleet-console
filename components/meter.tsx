import { cn } from "@/lib/utils";

export function Meter({
  value,
  warnAt = 85,
  label,
}: {
  value: number | null;
  warnAt?: number;
  label: string;
}) {
  const n = value == null ? null : Math.max(0, Math.min(100, value));
  const hot = n != null && n >= warnAt;
  return (
    <div className="min-w-[7.5rem]">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-[10px] tracking-wide text-muted-foreground uppercase">
          {label}
        </span>
        <span className="tabular font-mono text-[11px] text-foreground/90">
          {n == null ? "—" : `${Math.round(n)}%`}
        </span>
      </div>
      <div className="mt-1 h-1 overflow-hidden rounded-full bg-white/8">
        <div
          className={cn(
            "h-full rounded-full transition-[width,background-color] duration-200 ease-out",
            n == null ? "w-0" : hot ? "bg-load" : "bg-online/80"
          )}
          style={{ width: n == null ? "0%" : `${n}%` }}
        />
      </div>
    </div>
  );
}
