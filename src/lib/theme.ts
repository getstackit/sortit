export const THEME_STORAGE_KEY = "splat-theme";

export type ThemePreference = "light" | "dark" | "system";

export const THEME_CYCLE: ThemePreference[] = ["light", "dark", "system"];

export function nextTheme(current: ThemePreference): ThemePreference {
  const index = THEME_CYCLE.indexOf(current);
  return THEME_CYCLE[(index + 1) % THEME_CYCLE.length];
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
