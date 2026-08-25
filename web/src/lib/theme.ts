export type ThemePref = "system" | "light" | "dark";

const KEY = "sorting-desk-theme";

export function readThemePref(): ThemePref {
  try {
    const value = localStorage.getItem(KEY);
    if (value === "light" || value === "dark" || value === "system") return value;
  } catch {
    /* ignore */
  }
  return "system";
}

export function applyThemePref(pref: ThemePref) {
  document.documentElement.dataset.theme = pref;
  try {
    localStorage.setItem(KEY, pref);
  } catch {
    /* ignore */
  }
}

export function cycleTheme(pref: ThemePref): ThemePref {
  if (pref === "system") return "light";
  if (pref === "light") return "dark";
  return "system";
}

export function themeLabel(pref: ThemePref) {
  if (pref === "light") return "明亮";
  if (pref === "dark") return "暗黑";
  return "跟随系统";
}
