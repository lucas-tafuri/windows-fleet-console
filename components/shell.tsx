"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { FolderKey, LayoutGrid, Monitor, Package } from "lucide-react";
import { cn } from "@/lib/utils";
import { JoinPrompt } from "@/components/join-prompt";

const NAV = [
  { href: "/", label: "Fleet", icon: Monitor },
  { href: "/software", label: "Software", icon: Package },
  { href: "/jobs", label: "Jobs", icon: LayoutGrid },
  { href: "/enroll", label: "Enroll", icon: FolderKey },
];

export function Shell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="flex min-h-full flex-col md:flex-row">
      <aside className="hidden border-r border-white/8 bg-sidebar md:flex md:w-52 md:flex-col">
        <div className="px-5 pt-6 pb-4">
          <p className="font-mono text-[10px] tracking-[0.28em] text-primary uppercase">
            Fleet
          </p>
          <h1 className="mt-1 text-lg font-medium tracking-tight">Console</h1>
          <p className="mt-1 text-xs text-muted-foreground">Windows ops</p>
        </div>
        <nav className="flex flex-1 flex-col gap-1 px-3">
          {NAV.map((item) => {
            const active =
              item.href === "/"
                ? pathname === "/"
                : pathname.startsWith(item.href);
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors duration-150",
                  active
                    ? "bg-accent text-primary"
                    : "text-muted-foreground hover:bg-white/5 hover:text-foreground"
                )}
              >
                <Icon className="size-4" />
                {item.label}
              </Link>
            );
          })}
        </nav>
        <p className="px-5 py-4 font-mono text-[10px] text-muted-foreground/80">
          Local control plane
        </p>
      </aside>

      <div className="flex min-h-full min-w-0 flex-1 flex-col pb-16 md:pb-0">
        {children}
      </div>

      <JoinPrompt />

      <nav className="fixed inset-x-0 bottom-0 z-40 flex border-t border-white/8 bg-[#161410]/95 backdrop-blur-md md:hidden">
        {NAV.map((item) => {
          const active =
            item.href === "/"
              ? pathname === "/"
              : pathname.startsWith(item.href);
          const Icon = item.icon;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex flex-1 flex-col items-center gap-1 py-2.5 text-[11px]",
                active ? "text-primary" : "text-muted-foreground"
              )}
            >
              <Icon className="size-4" />
              {item.label}
            </Link>
          );
        })}
      </nav>
    </div>
  );
}
