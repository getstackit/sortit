export const THEME_STORAGE_KEY = "splat-theme";

export const themeInitScript = `
(() => {
  try {
    const stored = window.localStorage.getItem("${THEME_STORAGE_KEY}");
    const dark =
      stored === "dark" ||
      (!stored &&
        window.matchMedia &&
        window.matchMedia("(prefers-color-scheme: dark)").matches);

    document.documentElement.classList.toggle("dark", dark);
  } catch {
    document.documentElement.classList.remove("dark");
  }
})();
`;
