"use client";

import { useSyncExternalStore } from "react";
import { Moon, Sun } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { THEME_STORAGE_KEY } from "@/lib/theme";

function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  window.addEventListener("themechange", callback);

  return () => {
    window.removeEventListener("storage", callback);
    window.removeEventListener("themechange", callback);
  };
}

function getSnapshot() {
  if (typeof document === "undefined") {
    return false;
  }

  return document.documentElement.classList.contains("dark");
}

export function ThemeToggle() {
  const dark = useSyncExternalStore(subscribe, getSnapshot, () => false);

  function toggle(next: boolean) {
    document.documentElement.classList.toggle("dark", next);
    window.localStorage.setItem(THEME_STORAGE_KEY, next ? "dark" : "light");
    window.dispatchEvent(new Event("themechange"));
  }

  return (
    <div className="flex items-center gap-2">
      <Sun className="h-3.5 w-3.5 text-muted-foreground" />
      <Switch checked={dark} onCheckedChange={toggle} />
      <Moon className="h-3.5 w-3.5 text-muted-foreground" />
    </div>
  );
}
