import type { SoftwareInstall } from "@/lib/types";

export function SoftwareCell({ value, title }: { value?: SoftwareInstall; title: string }) {
  if (!value) return <span className="text-sm text-slate-400">Not checked</span>;
  const entries = [...(value.installations || [])].sort((a, b) =>
    (b.version || "").replace(/^\D+/, "").localeCompare((a.version || "").replace(/^\D+/, ""), undefined, { numeric: true })
  );
  return (
    <div className="space-y-3">
      {value.installed ? entries.length ? (
        <div className="flex flex-wrap items-center gap-2" aria-label={`${title}: installed`}>
          <span className="size-1.5 shrink-0 rounded-full bg-emerald-300" aria-hidden="true" />
          {entries.slice(0, 1).map((entry, index) => (
            <span key={index} className="break-all rounded-md border border-white/15 bg-white/5 px-2.5 py-1 font-mono text-base font-semibold leading-6 text-slate-50">
              {entry.version || "Version unknown"}
            </span>
          ))}
        </div>
      ) : <span className="inline-flex items-center gap-2 text-sm text-slate-200"><span className="size-1.5 rounded-full bg-emerald-300" />Installed · version unknown</span>
        : <span className="inline-block rounded-md bg-amber-400/10 px-2.5 py-1 text-sm font-medium text-amber-200">Not installed</span>}
      <details className="text-sm">
        <summary className="w-fit cursor-pointer rounded text-slate-300 underline-offset-4 hover:text-white hover:underline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary">
          {entries.length > 1 ? `All ${entries.length} installations` : "Details"}
        </summary>
        <div className="mt-3 space-y-3 border-l-2 border-white/15 pl-3">
          {entries.map((entry, index) => (
            <div key={index}>
              <p className="leading-5 text-slate-300">{entry.name}</p>
              <p className="mt-0.5 break-all font-mono font-medium text-white">{entry.version || "Version unavailable"}</p>
            </div>
          ))}
          {value.installed && !entries.length ? <p className="max-w-64 leading-5 text-slate-300">Update the client agent and check again to retrieve its versions.</p> : null}
          <p className="text-xs leading-5 text-slate-400">Checked {new Date(value.lastChecked).toLocaleString()}{value.via ? ` · ${value.via}` : ""}</p>
        </div>
      </details>
    </div>
  );
}
