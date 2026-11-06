import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { preferencesStorageKey, savePreferences, defaultPreferences } from "./preferences";
import { applyAppearance, applyStoredAppearance, pageAppearance, storedAppearance } from "./theme";

class MemoryStorage {
  values = new Map<string, string>();
  getItem(key: string): string | null { return this.values.get(key) ?? null; }
  setItem(key: string, value: string): void { this.values.set(key, value); }
}

const blocked = { getItem() { throw new Error("blocked"); } };
const themeColor = () => document.querySelector('meta[name="theme-color"]')!.getAttribute("content");

describe("startup appearance", () => {
  let meta: HTMLMetaElement;
  beforeEach(() => {
    document.documentElement.removeAttribute("data-theme");
    meta = document.createElement("meta");
    meta.name = "theme-color";
    meta.content = "#efe2dc";
    document.head.append(meta);
  });
  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    meta.remove();
  });

  it("sets the root to a stored Dark choice before React renders", () => {
    const storage = new MemoryStorage();
    savePreferences({ ...defaultPreferences, appearance: "dark" }, storage);
    expect(applyStoredAppearance(storage)).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(pageAppearance()).toBe("dark");
    expect(themeColor()).toBe("#22070a");
  });

  it("selects Light when nothing is stored, the stored value is invalid, or storage cannot be read", () => {
    expect(applyStoredAppearance(new MemoryStorage())).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");

    const invalid = new MemoryStorage();
    invalid.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, appearance: "night" }));
    expect(applyStoredAppearance(invalid)).toBe("light");

    document.documentElement.setAttribute("data-theme", "dark");
    expect(applyStoredAppearance(blocked)).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(themeColor()).toBe("#efe2dc");
  });

  it("lets Light override a Dark root that was already set", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    applyAppearance("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(storedAppearance(blocked)).toBe("light");
  });

  it("reads an unknown or absent page appearance as Light", () => {
    expect(pageAppearance()).toBe("light");
    document.documentElement.setAttribute("data-theme", "system");
    expect(pageAppearance()).toBe("light");
  });
});
