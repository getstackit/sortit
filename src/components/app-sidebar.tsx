"use client";

import { useState } from "react";
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
        "flex items-center rounded-md px-2 py-2 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
        active && "bg-sidebar-accent font-medium text-sidebar-accent-foreground",
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
}: AppSidebarProps) {
  const pathname = usePathname();
  const { collapsed, closeMobileSidebar } = useAppShell();
  const [pasteOpen, setPasteOpen] = useState(false);

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-sidebar-border px-3 py-3">
        <div
          className={cn(
            "flex items-center gap-2",
            collapsed && "justify-center"
          )}
        >
          <span className="text-lg font-semibold tracking-tight">s</span>
          <span className={cn("text-sm font-medium", collapsed && "hidden")}>
            splat
          </span>
        </div>
      </div>

      <div className="px-3 pt-3">
        <button
          type="button"
          onClick={() => setPasteOpen(true)}
          className={cn(
            "flex w-full items-center gap-2 rounded-md bg-primary px-2 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90",
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
        open={pasteOpen}
        onOpenChange={setPasteOpen}
        onIssueCreated={onIssueCreated}
        navigateOnCreate={navigateOnCreate}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 py-4">
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
              <p className="px-2 py-1 text-xs text-sidebar-foreground/50">
                Nothing yet
              </p>
            )}
            {things.map((thing, index) => (
              <Link
                key={thing.id}
                href={thing.href ?? `#${thing.id}`}
                title={thing.title}
                onClick={closeMobileSidebar}
                className={cn(
                  "flex items-center rounded-md px-2 py-2 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
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
            ))}
          </div>
        </section>
      </div>

      <div className="border-t border-sidebar-border px-3 py-3">
        <div className={cn(collapsed && "px-0")}>
          <ThemeToggle />
        </div>
      </div>
    </div>
  );
}
