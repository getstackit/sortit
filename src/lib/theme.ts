export const THEME_STORAGE_KEY = "sortit-theme";

export type ThemePreference = "light" | "dark" | "system";

export const THEME_CYCLE: ThemePreference[] = ["light", "dark", "system"];

export function nextTheme(current: ThemePreference): ThemePreference {
  const index = THEME_CYCLE.indexOf(current);
  return THEME_CYCLE[(index + 1) % THEME_CYCLE.length];
}

/** Reads the persisted theme preference, defaulting to "system". Client-only. */
export function getStoredTheme(): ThemePreference {
  if (typeof window === "undefined") {
    return "system";
  }

  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  return "system";
}

/** Applies a theme preference to the document and persists it. Client-only. */
export function applyTheme(preference: ThemePreference) {
  const dark =
    preference === "system"
      ? window.matchMedia("(prefers-color-scheme: dark)").matches
      : preference === "dark";

  document.documentElement.classList.toggle("dark", dark);
  window.localStorage.setItem(THEME_STORAGE_KEY, preference);
  window.dispatchEvent(new Event("themechange"));
}

/** Cycles to the next theme preference. Shared by the toggle button and palette. */
export function toggleTheme() {
  applyTheme(nextTheme(getStoredTheme()));
}

export const themeInitScript = `
(() => {
  try {
    const stored = window.localStorage.getItem("${THEME_STORAGE_KEY}");
    const prefersDark =
      window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;

    const dark =
      stored === "dark" || (stored !== "light" && prefersDark);

    document.documentElement.classList.toggle("dark", dark);
  } catch {
    document.documentElement.classList.remove("dark");
  }
})();
`;
