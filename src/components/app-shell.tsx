"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { useIsMobile } from "@/hooks/use-mobile";

const DESKTOP_SIDEBAR_STORAGE_KEY = "bored-sidebar-collapsed";

type AppShellContextValue = {
  collapsed: boolean;
  isMobile: boolean;
  mobileOpen: boolean;
  toggleSidebar: () => void;
  closeMobileSidebar: () => void;
};

const AppShellContext = createContext<AppShellContextValue | null>(null);

export function useAppShell() {
  const context = useContext(AppShellContext);

  if (!context) {
    throw new Error("useAppShell must be used within an AppShell.");
  }

  return context;
}

export function AppShell({
  sidebar,
  children,
}: {
  sidebar: ReactNode;
  children: ReactNode;
}) {
  const isMobile = useIsMobile();
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }

    return window.localStorage.getItem(DESKTOP_SIDEBAR_STORAGE_KEY) === "true";
  });
  const [mobileOpen, setMobileOpen] = useState(false);

  useEffect(() => {
    window.localStorage.setItem(
      DESKTOP_SIDEBAR_STORAGE_KEY,
      collapsed ? "true" : "false"
    );
  }, [collapsed]);

  const mobileSidebarOpen = isMobile && mobileOpen;

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key !== "b" || (!event.metaKey && !event.ctrlKey)) {
        return;
      }

      event.preventDefault();
      if (isMobile) {
        setMobileOpen((open) => !open);
        return;
      }

      setCollapsed((value) => !value);
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isMobile]);

  const value = useMemo<AppShellContextValue>(
    () => ({
      collapsed,
      isMobile,
      mobileOpen,
      toggleSidebar() {
        if (isMobile) {
          setMobileOpen((open) => !open);
          return;
        }

        setCollapsed((open) => !open);
      },
      closeMobileSidebar() {
        setMobileOpen(false);
      },
    }),
    [collapsed, isMobile, mobileOpen]
  );

  return (
    <AppShellContext.Provider value={value}>
      <div className="h-svh overflow-hidden bg-sidebar">
        <div className="flex h-full">
          <button
            type="button"
            aria-label="Close sidebar"
            className={cn(
              "fixed inset-0 z-30 bg-black/35 transition-opacity md:hidden",
              mobileSidebarOpen ? "opacity-100" : "pointer-events-none opacity-0"
            )}
            onClick={() => setMobileOpen(false)}
          />
          <aside
            className={cn(
              "fixed inset-y-0 left-0 z-40 flex w-72 flex-col border-r bg-sidebar text-sidebar-foreground transition-transform duration-200 md:static md:z-auto md:m-2 md:mr-0 md:h-[calc(100svh-1rem)] md:translate-x-0 md:rounded-l-xl md:border md:shadow-sm",
              mobileSidebarOpen ? "translate-x-0" : "-translate-x-full",
              collapsed && "md:w-14"
            )}
          >
            {sidebar}
          </aside>
          <main className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-background md:m-2 md:ml-0 md:h-[calc(100svh-1rem)] md:rounded-r-xl md:shadow-sm">
            {children}
          </main>
        </div>
      </div>
    </AppShellContext.Provider>
  );
}

export function AppShellToggle({
  className,
}: {
  className?: string;
}) {
  const { toggleSidebar } = useAppShell();

  return (
    <button
      type="button"
      aria-label="Toggle sidebar"
      onClick={toggleSidebar}
      className={cn(
        "inline-flex size-8 items-center justify-center rounded-md border border-transparent text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        className
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
        <path d="M2.5 3.5h11" />
        <path d="M2.5 8h11" />
        <path d="M2.5 12.5h11" />
      </svg>
    </button>
  );
}
