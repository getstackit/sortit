"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAppShell } from "@/components/app-shell";
import { useAuth } from "@/components/auth-provider";
import { PasteModal } from "@/components/paste-modal";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import type { IssueRecord } from "@/lib/issues";
import { NAV_GROUPS, SHOW_DEV_NAV, navItemsForGroup } from "@/lib/nav";
import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";

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

function NavLink({
  href,
  label,
  icon: Icon,
  active,
  collapsed,
  onClick,
  className,
  children,
}: {
  href: string;
  label: string;
  icon?: LucideIcon;
  active?: boolean;
  collapsed: boolean;
  onClick: () => void;
  className?: string;
  children?: React.ReactNode;
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
        collapsed && "justify-center px-0",
        className
      )}
    >
      {children ?? (
        <>
          {Icon && <Icon className={cn("size-4 shrink-0", !collapsed && "mr-2")} />}
          <span className={cn("truncate", collapsed && "sr-only")}>{label}</span>
        </>
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
  const { user, logout } = useAuth();
  const { collapsed, closeMobileSidebar, composerOpen, openComposer, setComposerOpen } =
    useAppShell();

  return (
    <div className="flex h-full flex-col">
      <SidebarHeader className="relative min-h-14 justify-center overflow-hidden border-b border-sidebar-border/70 p-3">
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
            sortit
          </span>
        </div>
        <div className="app-gradient-rule animate-gradient-shift absolute inset-x-0 bottom-0 h-px opacity-75" />
      </SidebarHeader>

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

      <SidebarContent className="app-scrollarea gap-5 px-3 py-4">
        {NAV_GROUPS.map((group) => {
          const items = navItemsForGroup(group.id, SHOW_DEV_NAV);
          if (items.length === 0) {
            return null;
          }

          return (
            <SidebarGroup key={group.id} className="p-0 space-y-2">
              <SidebarGroupLabel
                className={cn(
                  "h-auto px-2 text-[11px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/50",
                  collapsed && "hidden"
                )}
              >
                {group.label}
              </SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu className="gap-1">
                  {items.map((item) => (
                    <SidebarMenuItem key={item.href}>
                      <NavLink
                        href={item.href}
                        label={item.label}
                        icon={item.icon}
                        active={pathname === item.href}
                        collapsed={collapsed}
                        onClick={closeMobileSidebar}
                      />
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          );
        })}

        {showThingsSection && things.length > 0 && (
          <SidebarGroup className="min-h-0 flex-1 p-0 space-y-2">
            <SidebarGroupLabel
              className={cn(
                "h-auto px-2 text-[11px] font-medium uppercase tracking-[0.16em] text-sidebar-foreground/50",
                collapsed && "hidden"
              )}
            >
              Things
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="gap-1">
                {things.map((thing, index) => {
                  const href = thing.href ?? `#${thing.id}`;
                  const isActive = pathname === href;
                  return (
                    <SidebarMenuItem key={thing.id}>
                      <NavLink
                        href={href}
                        label={thing.title}
                        active={isActive}
                        collapsed={collapsed}
                        onClick={closeMobileSidebar}
                        className={cn(
                          "border-l-[3px] border-l-transparent",
                          isActive && "border-l-[var(--glow-color-current)]"
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
                      </NavLink>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter className="border-t border-sidebar-border p-3">
        {!collapsed && user && (
          <div className="rounded-2xl border border-sidebar-border/70 bg-sidebar-accent/55 px-3 py-3">
            <p className="text-sm font-medium text-sidebar-accent-foreground">
              {user.displayName}
            </p>
            <p className="mt-1 text-xs text-sidebar-foreground/60">@{user.login}</p>
            <button
              type="button"
              onClick={() => void logout()}
              className="mt-3 rounded-xl border border-sidebar-border/70 px-3 py-2 text-xs font-medium text-sidebar-accent-foreground transition-colors hover:bg-sidebar-accent/80"
            >
              Sign out
            </button>
          </div>
        )}
        <div className={cn(collapsed && "px-0")}>
          <ThemeToggle />
        </div>
      </SidebarFooter>
    </div>
  );
}
