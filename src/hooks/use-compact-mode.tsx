"use client";

import {
  createContext,
  useCallback,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";

const STORAGE_KEY = "splat-compact-mode";

function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  return () => window.removeEventListener("storage", callback);
}

function getSnapshot(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(STORAGE_KEY) === "true";
}

const CompactContext = createContext<{
  isCompact: boolean;
  toggleCompact: () => void;
}>({ isCompact: false, toggleCompact: () => {} });

export function CompactModeProvider({ children }: { children: ReactNode }) {
  const isCompact = useSyncExternalStore(subscribe, getSnapshot, () => false);

  const toggleCompact = useCallback(() => {
    const next = !getSnapshot();
    window.localStorage.setItem(STORAGE_KEY, String(next));
    window.dispatchEvent(new Event("storage"));
  }, []);

  return (
    <CompactContext.Provider value={{ isCompact, toggleCompact }}>
      {children}
    </CompactContext.Provider>
  );
}

export function useCompactMode() {
  return useContext(CompactContext);
}
