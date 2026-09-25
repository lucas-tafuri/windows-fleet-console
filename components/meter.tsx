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
  const n = value == null || !Number.isFinite(value) ? null : Math.max(0, Math.min(100, value));
  const hot = n != null && n >= warnAt;
  return (
    <div className="min-w-0" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={n ?? undefined} aria-valuetext={n == null ? "Unavailable" : `${Math.round(n)} percent`}>
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-[10px] tracking-wide text-muted-foreground uppercase">
          {label}
        </span>
        <span className="tabular font-mono text-sm font-medium text-foreground/90">
          {n == null ? "—" : `${Math.round(n)}%`}
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/8">
        <div
          className={cn(
            "h-full rounded-full transition-[width,background-color] duration-200 ease-out",
            n == null ? "w-0" : hot ? "bg-load" : label === "GPU" ? "bg-violet-400" : label === "RAM" ? "bg-sky-400" : "bg-online"
          )}
          style={{ width: n == null ? "0%" : `${n}%` }}
        />
      </div>
    </div>
  );
}
