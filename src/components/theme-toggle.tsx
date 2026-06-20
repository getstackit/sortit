"use client";

import { useSyncExternalStore } from "react";
import { MonitorIcon, MoonStarIcon, SunMediumIcon } from "lucide-react";
import { getStoredTheme, toggleTheme, type ThemePreference } from "@/lib/theme";

function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  window.addEventListener("themechange", callback);

  return () => {
    window.removeEventListener("storage", callback);
    window.removeEventListener("themechange", callback);
  };
}

const ICONS: Record<ThemePreference, typeof SunMediumIcon> = {
  light: SunMediumIcon,
  dark: MoonStarIcon,
  system: MonitorIcon,
};

export function ThemeToggle() {
  const preference = useSyncExternalStore(
    subscribe,
    getStoredTheme,
    () => "system" as ThemePreference
  );
  const Icon = ICONS[preference];

  return (
    <button
      type="button"
      onClick={toggleTheme}
      aria-pressed={preference === "dark"}
      className="flex w-full items-center justify-between rounded-xl border border-border/60 bg-background/76 px-3 py-2.5 text-left text-xs transition-all hover:border-border hover:bg-background/92 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      <span className="flex items-center gap-2 font-medium text-foreground">
        <span className="flex size-6 items-center justify-center rounded-full bg-muted/70 text-muted-foreground">
          <Icon className="size-3.5" />
        </span>
        Theme
      </span>
      <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
        {preference}
      </span>
    </button>
  );
}
