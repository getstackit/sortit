"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAppShell } from "@/components/app-shell";
import { PasteModal } from "@/components/paste-modal";
import { ThemeToggle } from "@/components/theme-toggle";
import type { IssueRecord } from "@/lib/issues";
import { cn } from "@/lib/utils";

type Thing = {
  id: string;
  title: string;
  href?: string;
};

type AppSidebarProps = {
  things?: Thing[];
  onIssueCreated?: (issue: IssueRecord) => Promise<void> | void;
  navigateOnCreate?: boolean;
  showThingsSection?: boolean;
};

function SidebarLink({
  href,
  label,
  active,
  collapsed,
  onClick,
}: {
  href: string;
  label: string;
  active?: boolean;
  collapsed: boolean;
  onClick: () => void;
}) {
  return (
    <Link
      href={href}
      title={label}
      onClick={onClick}
      className={cn(
        "flex items-center rounded-xl border border-transparent px-2.5 py-2 text-sm transition-all hover:border-sidebar-border/70 hover:bg-sidebar-accent/80 hover:text-sidebar-accent-foreground",
        active &&
          "border-sidebar-border/70 bg-sidebar-accent/90 font-medium text-sidebar-accent-foreground shadow-sm",
        collapsed && "justify-center px-0"
      )}
    >
      <span className={cn("truncate", collapsed && "sr-only")}>{label}</span>
      {collapsed && (
        <span aria-hidden="true" className="text-[11px] font-medium uppercase">
          {label.charAt(0)}
        </span>
      )}
    </Link>
  );
}

export function AppSidebar({
  things = [],
  onIssueCreated,
  navigateOnCreate = true,
  showThingsSection = true,
}: AppSidebarProps) {
  const pathname = usePathname();
  const { collapsed, closeMobileSidebar, composerOpen, openComposer, setComposerOpen } =
    useAppShell();

  return (
    <div className="flex h-full flex-col">
      <div className="relative overflow-hidden border-b border-sidebar-border/70 px-3 py-3">
        <div
          className={cn(
            "flex items-center gap-2",
            collapsed && "justify-center"
          )}
        >
          <span
            className="flex size-8 items-center justify-center rounded-2xl text-sm font-semibold text-white shadow-sm"
            style={{
              background:
                "linear-gradient(135deg, var(--gradient-start), var(--gradient-mid) 55%, var(--gradient-end))",
            }}
          >
            s
          </span>
          <span className={cn("text-sm font-medium", collapsed && "hidden")}>
            splat
          </span>
        </div>
        <div className="app-gradient-rule animate-gradient-shift absolute inset-x-0 bottom-0 h-px opacity-75" />
      </div>

      <div className="px-3 pt-3">
        <button
          type="button"
          aria-keyshortcuts="N"
          onClick={openComposer}
          className={cn(
            "flex w-full items-center gap-2 rounded-xl border border-border/60 bg-sidebar-accent px-2.5 py-2.5 text-sm font-medium text-sidebar-accent-foreground transition-all hover:bg-sidebar-accent/80 hover:shadow-sm",
            collapsed && "justify-center px-0"
          )}
        >
          <svg
            aria-hidden="true"
            viewBox="0 0 16 16"
            className="size-4"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
          >
            <path d="M8 3v10" />
            <path d="M3 8h10" />
          </svg>
          <span className={cn(collapsed && "sr-only")}>Add</span>
        </button>
      </div>

      <PasteModal
        open={composerOpen}
        onOpenChange={setComposerOpen}
        onIssueCreated={onIssueCreated}
        navigateOnCreate={navigateOnCreate}
      />

      <div className="app-scrollarea flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 py-4">
        <section className="space-y-2">
          <p
            className={cn(
              "px-2 text-[11px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/50",
              collapsed && "hidden"
            )}
          >
            Views
          </p>
          <div className="space-y-1">
            <SidebarLink
              href="/"
              label="All Issues"
              active={pathname === "/"}
              collapsed={collapsed}
              onClick={closeMobileSidebar}
            />
            <SidebarLink
              href="/map"
              label="Map"
              active={pathname === "/map"}
              collapsed={collapsed}
              onClick={closeMobileSidebar}
            />
            <SidebarLink
              href="/tags"
              label="Tag Map"
              active={pathname === "/tags"}
              collapsed={collapsed}
              onClick={closeMobileSidebar}
            />
            <SidebarLink
              href="/debug"
              label="Debug"
              active={pathname === "/debug"}
              collapsed={collapsed}
              onClick={closeMobileSidebar}
            />
          </div>
        </section>

        {showThingsSection && (
          <section className="min-h-0 flex-1 space-y-2">
            <p
              className={cn(
                "px-2 text-[11px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/50",
                collapsed && "hidden"
              )}
            >
              Things
            </p>
            <div className="space-y-1">
              {things.length === 0 && !collapsed && (
                <p className="rounded-xl px-2.5 py-2 text-xs text-sidebar-foreground/50">
                  Nothing yet
                </p>
              )}
              {things.map((thing, index) => {
                const isActive = pathname === (thing.href ?? `#${thing.id}`);
                return (
                <Link
                  key={thing.id}
                  href={thing.href ?? `#${thing.id}`}
                  title={thing.title}
                  onClick={closeMobileSidebar}
                  className={cn(
                    "flex items-center rounded-xl border-l-[3px] border-l-transparent border border-transparent px-2.5 py-2 text-sm transition-all hover:border-sidebar-border/70 hover:bg-sidebar-accent/80 hover:text-sidebar-accent-foreground",
                    isActive &&
                      "border-l-[var(--glow-color-current)] bg-sidebar-accent/90 font-medium text-sidebar-accent-foreground",
                    collapsed && "justify-center px-0"
                  )}
                >
                  <span className={cn("truncate", collapsed && "hidden")}>
                    {thing.title}
                  </span>
                  {collapsed && (
                    <span
                      aria-hidden="true"
                      className="text-[11px] font-medium uppercase text-sidebar-foreground/70"
                    >
                      {(index + 1).toString(36)}
                    </span>
                  )}
                </Link>
                );
              })}
            </div>
          </section>
        )}
      </div>

      <div className="border-t border-sidebar-border px-3 py-3">
        <div className={cn(collapsed && "px-0")}>
          <ThemeToggle />
        </div>
      </div>
    </div>
  );
}
