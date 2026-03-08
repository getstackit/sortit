"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useRouter } from "next/navigation";
import { Dialog } from "@base-ui/react/dialog";
import { cn } from "@/lib/utils";
import { useIsMobile } from "@/hooks/use-mobile";

const DESKTOP_SIDEBAR_STORAGE_KEY = "splat-sidebar-collapsed";
const SHORTCUT_SEQUENCE_TIMEOUT_MS = 1200;

const GLOBAL_SHORTCUTS = [
  { key: "Cmd/Ctrl+B", description: "Toggle the sidebar" },
  { key: "N", description: "Open the new issue composer" },
  { key: "G I", description: "Go to All Issues" },
  { key: "G M", description: "Go to Map" },
  { key: "G T", description: "Go to Tag Map" },
  { key: "G D", description: "Go to Debug" },
  { key: "?", description: "Show keyboard shortcuts" },
] as const;

export type AppShortcutRegistration = {
  key: string;
  description: string;
  action: () => void | Promise<void>;
};

type AppShellContextValue = {
  collapsed: boolean;
  isMobile: boolean;
  mobileOpen: boolean;
  composerOpen: boolean;
  shortcutHelpOpen: boolean;
  toggleSidebar: () => void;
  closeMobileSidebar: () => void;
  openComposer: () => void;
  setComposerOpen: (open: boolean) => void;
  openShortcutHelp: () => void;
};

const AppShellContext = createContext<AppShellContextValue | null>(null);

function isEditableElement(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  if (target.isContentEditable) {
    return true;
  }

  return target.closest("input, textarea, select, [contenteditable], [role='textbox']") !== null;
}

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
  shortcuts = [],
}: {
  sidebar: ReactNode;
  children: ReactNode;
  shortcuts?: AppShortcutRegistration[];
}) {
  const router = useRouter();
  const isMobile = useIsMobile();
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }

    return window.localStorage.getItem(DESKTOP_SIDEBAR_STORAGE_KEY) === "true";
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const [composerOpen, setComposerOpen] = useState(false);
  const [shortcutHelpOpen, setShortcutHelpOpen] = useState(false);
  const shortcutPrefixRef = useRef<string | null>(null);
  const shortcutTimeoutRef = useRef<number | null>(null);

  useEffect(() => {
    window.localStorage.setItem(
      DESKTOP_SIDEBAR_STORAGE_KEY,
      collapsed ? "true" : "false"
    );
  }, [collapsed]);

  const mobileSidebarOpen = isMobile && mobileOpen;

  const resetShortcutPrefix = useCallback(() => {
    shortcutPrefixRef.current = null;

    if (shortcutTimeoutRef.current !== null) {
      window.clearTimeout(shortcutTimeoutRef.current);
      shortcutTimeoutRef.current = null;
    }
  }, []);

  const startShortcutPrefix = useCallback(
    (prefix: string) => {
      resetShortcutPrefix();
      shortcutPrefixRef.current = prefix;
      shortcutTimeoutRef.current = window.setTimeout(() => {
        shortcutPrefixRef.current = null;
        shortcutTimeoutRef.current = null;
      }, SHORTCUT_SEQUENCE_TIMEOUT_MS);
    },
    [resetShortcutPrefix]
  );

  const navigateTo = useCallback(
    (href: string) => {
      resetShortcutPrefix();
      setComposerOpen(false);
      setShortcutHelpOpen(false);
      setMobileOpen(false);
      router.push(href);
    },
    [resetShortcutPrefix, router]
  );

  const openComposer = useCallback(() => {
    resetShortcutPrefix();
    setShortcutHelpOpen(false);
    setComposerOpen(true);
  }, [resetShortcutPrefix]);

  const openShortcutHelp = useCallback(() => {
    resetShortcutPrefix();
    setComposerOpen(false);
    setShortcutHelpOpen(true);
  }, [resetShortcutPrefix]);

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || event.repeat) {
        return;
      }

      const key = event.key.toLowerCase();
      const hasCommandModifier = event.metaKey || event.ctrlKey;
      const overlayOpen = composerOpen || shortcutHelpOpen;
      const typing = isEditableElement(event.target);

      if (
        key === "b" &&
        hasCommandModifier &&
        !overlayOpen &&
        !typing &&
        !event.altKey &&
        !event.shiftKey
      ) {
        event.preventDefault();
        resetShortcutPrefix();

        if (isMobile) {
          setMobileOpen((open) => !open);
          return;
        }

        setCollapsed((value) => !value);
        return;
      }

      if (typing) {
        resetShortcutPrefix();
        return;
      }

      if (shortcutPrefixRef.current === "g") {
        resetShortcutPrefix();

        if (hasCommandModifier || event.altKey) {
          return;
        }

        const destinations: Record<string, string> = {
          i: "/",
          m: "/map",
          t: "/tags",
          d: "/debug",
        };

        const href = destinations[key];
        if (!href || overlayOpen) {
          return;
        }

        event.preventDefault();
        navigateTo(href);
        return;
      }

      if (overlayOpen || hasCommandModifier || event.altKey) {
        resetShortcutPrefix();
        return;
      }

      if (event.key === "?") {
        event.preventDefault();
        openShortcutHelp();
        return;
      }

      if (key === "g") {
        event.preventDefault();
        startShortcutPrefix("g");
        return;
      }

      if (key === "n") {
        event.preventDefault();
        openComposer();
        return;
      }

      const pageShortcut = shortcuts.find(
        (shortcut) => shortcut.key.toLowerCase() === key
      );

      if (!pageShortcut) {
        return;
      }

      event.preventDefault();
      void pageShortcut.action();
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [
    composerOpen,
    isMobile,
    navigateTo,
    openComposer,
    openShortcutHelp,
    resetShortcutPrefix,
    shortcuts,
    shortcutHelpOpen,
    startShortcutPrefix,
  ]);

  useEffect(() => {
    if (!composerOpen && !shortcutHelpOpen) {
      return;
    }

    resetShortcutPrefix();
  }, [composerOpen, resetShortcutPrefix, shortcutHelpOpen]);

  useEffect(() => {
    return () => {
      if (shortcutTimeoutRef.current !== null) {
        window.clearTimeout(shortcutTimeoutRef.current);
      }
    };
  }, []);

  const value = useMemo<AppShellContextValue>(
    () => ({
      collapsed,
      isMobile,
      mobileOpen,
      composerOpen,
      shortcutHelpOpen,
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
      openComposer,
      setComposerOpen,
      openShortcutHelp,
    }),
    [
      collapsed,
      composerOpen,
      isMobile,
      mobileOpen,
      openComposer,
      openShortcutHelp,
      shortcutHelpOpen,
    ]
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
      <Dialog.Root open={shortcutHelpOpen} onOpenChange={setShortcutHelpOpen}>
        <Dialog.Portal>
          <Dialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
          <Dialog.Viewport className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto px-4 py-[10vh]">
            <Dialog.Popup className="w-full max-w-2xl rounded-2xl border bg-background p-5 shadow-lg transition-all data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <Dialog.Title className="text-sm font-medium">
                    Keyboard shortcuts
                  </Dialog.Title>
                  <Dialog.Description className="mt-1 text-xs text-muted-foreground">
                    Single-key shortcuts pause while you are typing in a field.
                  </Dialog.Description>
                </div>
                <Dialog.Close className="rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground">
                  Close
                </Dialog.Close>
              </div>

              <div className="mt-5 grid gap-6 md:grid-cols-2">
                <section className="space-y-3">
                  <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                    Global
                  </p>
                  <div className="space-y-2">
                    {GLOBAL_SHORTCUTS.map((shortcut) => (
                      <div
                        key={shortcut.key}
                        className="flex items-center justify-between gap-4 rounded-xl border border-border/60 bg-card px-3 py-2"
                      >
                        <span className="text-sm text-foreground">
                          {shortcut.description}
                        </span>
                        <kbd className="rounded-md border border-border bg-background px-2 py-1 text-[11px] font-medium text-muted-foreground">
                          {shortcut.key}
                        </kbd>
                      </div>
                    ))}
                  </div>
                </section>

                {shortcuts.length > 0 && (
                  <section className="space-y-3">
                    <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                      This page
                    </p>
                    <div className="space-y-2">
                      {shortcuts.map((shortcut) => (
                        <div
                          key={shortcut.key}
                          className="flex items-center justify-between gap-4 rounded-xl border border-border/60 bg-card px-3 py-2"
                        >
                          <span className="text-sm text-foreground">
                            {shortcut.description}
                          </span>
                          <kbd className="rounded-md border border-border bg-background px-2 py-1 text-[11px] font-medium text-muted-foreground">
                            {shortcut.key.toUpperCase()}
                          </kbd>
                        </div>
                      ))}
                    </div>
                  </section>
                )}
              </div>
            </Dialog.Popup>
          </Dialog.Viewport>
        </Dialog.Portal>
      </Dialog.Root>
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
      aria-keyshortcuts="Meta+B Control+B"
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
