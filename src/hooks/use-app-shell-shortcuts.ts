"use client";

import { useEffect, useEffectEvent, useRef } from "react";
import type { AppShortcutRegistration } from "@/components/app-shell";
import { NAV_ITEMS } from "@/lib/nav";

const SHORTCUT_SEQUENCE_TIMEOUT_MS = 1200;

/** `G <key>` go-to destinations, derived from the shared nav config. */
const NAV_DESTINATIONS: Record<string, string> = Object.fromEntries(
  NAV_ITEMS.filter((item) => item.shortcut).map((item) => [item.shortcut!, item.href])
);

const NAV_SHORTCUTS = NAV_ITEMS.filter((item) => item.shortcut).map((item) => ({
  key: `G ${item.shortcut!.toUpperCase()}`,
  description: `Go to ${item.label}`,
}));

export const GLOBAL_SHORTCUTS: { key: string; description: string }[] = [
  { key: "Cmd/Ctrl+B", description: "Toggle the sidebar" },
  { key: "N", description: "Open the new issue composer" },
  ...NAV_SHORTCUTS,
  { key: "/", description: "Open command palette" },
  { key: "?", description: "Show keyboard shortcuts" },
];

type UseAppShellShortcutsOptions = {
  commandPaletteOpen: boolean;
  composerOpen: boolean;
  isMobile: boolean;
  navigateTo: (href: string) => void;
  openCommandPalette: () => void;
  openComposer: () => void;
  openShortcutHelp: () => void;
  shortcutHelpOpen: boolean;
  shortcuts: AppShortcutRegistration[];
  toggleSidebar: () => void;
};

function isEditableElement(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  if (target.isContentEditable) {
    return true;
  }

  return (
    target.closest("input, textarea, select, [contenteditable], [role='textbox']") !==
    null
  );
}

export function useAppShellShortcuts({
  commandPaletteOpen,
  composerOpen,
  isMobile,
  navigateTo,
  openCommandPalette,
  openComposer,
  openShortcutHelp,
  shortcutHelpOpen,
  shortcuts,
  toggleSidebar,
}: UseAppShellShortcutsOptions) {
  const shortcutPrefixRef = useRef<string | null>(null);
  const shortcutTimeoutRef = useRef<number | null>(null);

  const resetShortcutPrefix = useEffectEvent(() => {
    shortcutPrefixRef.current = null;

    if (shortcutTimeoutRef.current !== null) {
      window.clearTimeout(shortcutTimeoutRef.current);
      shortcutTimeoutRef.current = null;
    }
  });

  const startShortcutPrefix = useEffectEvent((prefix: string) => {
    resetShortcutPrefix();
    shortcutPrefixRef.current = prefix;
    shortcutTimeoutRef.current = window.setTimeout(() => {
      shortcutPrefixRef.current = null;
      shortcutTimeoutRef.current = null;
    }, SHORTCUT_SEQUENCE_TIMEOUT_MS);
  });

  const handleKeyDown = useEffectEvent((event: KeyboardEvent) => {
    if (event.defaultPrevented || event.repeat) {
      return;
    }

    const key = event.key.toLowerCase();
    const hasCommandModifier = event.metaKey || event.ctrlKey;
    const overlayOpen = composerOpen || shortcutHelpOpen || commandPaletteOpen;
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
      toggleSidebar();
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

      const href = NAV_DESTINATIONS[key];
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

    if (event.key === "/") {
      event.preventDefault();
      openCommandPalette();
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
  });

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      handleKeyDown(event);
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    if (!composerOpen && !shortcutHelpOpen && !commandPaletteOpen) {
      return;
    }

    resetShortcutPrefix();
  }, [commandPaletteOpen, composerOpen, shortcutHelpOpen]);

  useEffect(() => {
    return () => {
      if (shortcutTimeoutRef.current !== null) {
        window.clearTimeout(shortcutTimeoutRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (!isMobile) {
      resetShortcutPrefix();
    }
  }, [isMobile]);
}
