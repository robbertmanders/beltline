import { readPreferences, type Appearance } from "./preferences";

/** The page root's attribute that the dark token block and the native color scheme follow. */
export const appearanceAttribute = "data-theme";

/** The browser's theme-color for each appearance; index.html sets the Light one before any script runs. */
const themeColors: Record<Appearance, string> = { light: "#efe2dc", dark: "#22070a" };

/** The appearance the browser stored, or Light when storage cannot be read. */
export function storedAppearance(storage?: Pick<Storage, "getItem">): Appearance {
  return readPreferences(storage)?.appearance ?? "light";
}

/** The appearance the page is showing now. The page is the truth for this visit, even when storage refused a write. */
export function pageAppearance(root: HTMLElement = document.documentElement): Appearance {
  return root.getAttribute(appearanceAttribute) === "dark" ? "dark" : "light";
}

/** Puts an appearance on the page root and the browser's theme-color; the CSS does the rest. */
export function applyAppearance(appearance: Appearance, root: HTMLElement = document.documentElement): void {
  root.setAttribute(appearanceAttribute, appearance);
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", themeColors[appearance]);
}

/** Runs before React renders, so a stored Dark choice never shows the app in Light first. */
export function applyStoredAppearance(storage?: Pick<Storage, "getItem">, root?: HTMLElement): Appearance {
  const appearance = storedAppearance(storage);
  applyAppearance(appearance, root);
  return appearance;
}
