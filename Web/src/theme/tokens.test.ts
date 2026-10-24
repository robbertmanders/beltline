import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(resolve(process.cwd(), "src/theme/tokens.css"), "utf8");

/** The declarations inside one rule, by its selector. Light is `:root`; Dark is the `data-theme` attribute's rule. */
function ruleDeclarations(selector: string): Record<string, string> {
  const start = css.indexOf(`${selector} {`);
  if (start < 0) throw new Error(`Missing rule ${selector}`);
  const body = css.slice(start + selector.length + 2, css.indexOf("}", start));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((match) => [match[1]!, match[2]!.trim()]));
}

const declarations = ruleDeclarations(":root");
const darkDeclarations = ruleDeclarations(':root[data-theme="dark"]');

// Reviewed against the shared controls the palette serves. Keep this inventory independent of tokens.css.
const sharedAcrossAppearances = new Set(["--screen", "--screen-text", "--screen-bar", "--gold-lamp"]);
const tints = Object.keys(declarations).filter(name => name.startsWith("--tint-"));
const isLiteralColour = (value: string) => /^#|^rgba?\(/.test(value);

/** Parses a hex or rgb() colour into 0–255 channels and an alpha. */
function parseColour(value: string): { r: number; g: number; b: number; a: number } {
  const hex = /^#([0-9a-f]{3,8})$/i.exec(value);
  if (hex) {
    const digits = hex[1]!.length <= 4 ? [...hex[1]!].map(digit => digit + digit).join("") : hex[1]!;
    const channel = (at: number) => parseInt(digits.slice(at, at + 2), 16);
    return { r: channel(0), g: channel(2), b: channel(4), a: digits.length === 8 ? channel(6) / 255 : 1 };
  }
  const rgb = /^rgba?\(([^)]+)\)$/.exec(value);
  if (rgb) {
    const [r, g, b, a = "1"] = rgb[1]!.split(",").map(part => part.trim());
    return { r: Number(r), g: Number(g), b: Number(b), a: Number(a) };
  }
  throw new Error(`Not a literal colour: ${value}`);
}

/** The colour a token takes in a theme, following var() aliases and falling back to Light where Dark does not redeclare it. */
function resolveToken(name: string, theme: "light" | "dark"): { r: number; g: number; b: number; a: number } {
  const value = (theme === "dark" && darkDeclarations[name]) || declarations[name];
  if (!value) throw new Error(`Missing token ${name}`);
  const alias = /^var\((--[\w-]+)/.exec(value);
  return alias ? resolveToken(alias[1]!, theme) : parseColour(value);
}

/** The colour a surface shows once a translucent colour is laid over it. */
function composite(top: { r: number; g: number; b: number; a: number }, below: { r: number; g: number; b: number; a: number }) {
  const mix = (over: number, under: number) => over * top.a + under * (1 - top.a);
  return { r: mix(top.r, below.r), g: mix(top.g, below.g), b: mix(top.b, below.b), a: 1 };
}

function luminance(colour: { r: number; g: number; b: number }): number {
  const linear = (channel: number) => {
    const c = channel / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * linear(colour.r) + 0.7152 * linear(colour.g) + 0.0722 * linear(colour.b);
}

/** WCAG contrast of a foreground against a background token in one theme, compositing translucent colours. The foreground may also be a literal colour. */
function contrast(foreground: string, background: string, theme: "light" | "dark"): number {
  // A translucent surface is seen over the paper beneath it.
  const surface = composite(resolveToken(background, theme), resolveToken("--paper", theme));
  const ink = composite(foreground.startsWith("--") ? resolveToken(foreground, theme) : parseColour(foreground), surface);
  const [light, dark] = [luminance(ink), luminance(surface)].sort((a, b) => b - a) as [number, number];
  return (light + 0.05) / (dark + 0.05);
}

describe("Theme CSS token port", () => {
  it("keeps Light as the default and Dark only behind the appearance attribute", () => {
    expect(css).toContain(":root {\n  color-scheme: light;");
    expect(css).toContain(':root[data-theme="dark"] {\n  color-scheme: dark;');
    expect(css).not.toMatch(/prefers-color-scheme\s*:\s*dark/);
    expect(declarations["--font-size-body"]).toBe("14px");
    expect(declarations["--font-size-mono"]).toBe("12px");
    expect(declarations["--font-size-tag"]).toBe("11px");
    expect(declarations["--font-size-caps"]).toBe("10.5px");
    expect(declarations["--font-size-chip"]).toBe("10.5px");
    expect(declarations["--font-size-field"]).toBe("13.5px");
    expect(declarations["--card-padding"]).toBe("18px");
    expect(declarations["--card-radius"]).toBe("12px");
    expect(declarations["--hairline-width"]).toBe("1px");
  });

  it("redeclares every literal colour role in Dark, and nothing Dark declares is unknown to Light", () => {
    const literalLight = Object.keys(declarations).filter(name => isLiteralColour(declarations[name]!) && !sharedAcrossAppearances.has(name));
    expect(literalLight.filter(name => darkDeclarations[name] === undefined)).toEqual([]);
    expect(Object.keys(darkDeclarations).filter(name => declarations[name] === undefined)).toEqual([]);
    expect(Object.keys(darkDeclarations).filter(name => !(name in declarations) || !(name in declarations))).toEqual([]);
  });

  it("matches Light's own surfaces with Dark's colour-scheme and keeps shared colours shared", () => {
    expect(darkDeclarations["--desk"]).toBe("#141B25");
    expect(darkDeclarations["--paper"]).toBe("#1B2431");
    expect(darkDeclarations["--card"]).toBe("#243142");
    expect(darkDeclarations["--ink"]).toBe("#E6EDF5");
    expect(darkDeclarations["--slate"]).toBe("#9DBBE4");
    for (const name of sharedAcrossAppearances) {
      expect(darkDeclarations[name], name).toBeUndefined();
    }
  });

  const textPairs: Array<[string, string, number]> = [
    ["--ink", "--desk", 4.5], ["--ink", "--paper", 4.5], ["--ink", "--side", 4.5], ["--ink", "--card", 4.5],
    ["--ink", "--segment", 4.5], ["--ink", "--grey-wash", 4.5], ["--ink", "--hover", 4.5], ["--ink", "--slate-wash", 4.5],
    ["--ink2", "--paper", 4.5], ["--ink2", "--card", 4.5], ["--ink2", "--side", 4.5],
    ["--muted", "--paper", 4.5], ["--muted", "--card", 4.5], ["--muted", "--side", 4.5],
    ["--tone-good-fg", "--tone-good-bg", 4.5], ["--tone-you-fg", "--tone-you-bg", 4.5],
    ["--tone-problem-fg", "--tone-problem-bg", 4.5], ["--tone-neutral-fg", "--tone-neutral-bg", 4.5],
    ["--status-done-fg", "--status-done-bg", 4.5], ["--status-running-fg", "--status-running-bg", 4.5],
    ["--status-held-fg", "--status-held-bg", 4.5], ["--status-waiting-fg", "--status-waiting-bg", 4.5],
    ["--status-plain-fg", "--status-plain-bg", 4.5],
    ["--gold-text", "--gold-wash", 4.5], ["--red-text", "--red-wash", 4.5], ["--red-text", "--error-wash", 4.5],
    ["--code-text", "--code-wash", 4.5], ["--ink2", "--code-wash", 4.5],
    ["--on-accent", "--ink", 4.5], ["--on-accent", "--slate", 4.5], ["--on-accent", "--red", 4.5],
    // The needs badge's ! sits on slate-text, which Dark sets to the bright slate.
    ["--on-accent", "--slate-text", 4.5],
    ["--trigger-caption", "--trigger-fill", 4.5], ["--trigger-detail", "--trigger-fill", 4.5],
    ["--screen-text", "--screen", 4.5], ["--slate-text", "--paper", 4.5], ["--slate-text", "--slate-wash", 4.5],
    ["--gold-text", "--gold-wash", 4.5], ["--gold-text-strong", "--gold-wash", 4.5], ["--red-text", "--error-wash", 4.5],
    ["--ink", "--row-needs", 4.5], ["--ink", "--row-held", 4.5], ["--ink", "--row-hover", 4.5],
    ["--sheet-ink", "--sheet-glass", 4.5], ["--sheet-body-ink", "--sheet-glass", 4.5], ["--sheet-button-ink", "--sheet-button", 4.5],
    ["--on-red", "--red", 4.5], ["--on-slate-deep", "--slate-deep", 4.5], ["--on-slate-deep", "--slate-hover", 4.5],
    ["--sheet-destructive-ink", "--sheet-destructive", 4.5], ["--ink2", "--card", 4.5], ["--muted", "--card", 4.5],
    // Slate text, which Dark uses where Light has slate-deep: links, labels, code, route names, buttons and chips.
    ["--slate-text", "--desk", 4.5], ["--slate-text", "--side", 4.5], ["--slate-text", "--card", 4.5], ["--slate-text", "--hover", 4.5],
    ["--slate-text", "--grey-wash", 4.5], ["--slate-text", "--segment", 4.5], ["--slate-text", "--fork-wash", 4.5],
    ["--slate-text", "--row-needs", 4.5], ["--slate-text", "--row-hover", 4.5], ["--card", "--slate", 4.5],
    // Buttons: their label on each fill, and the default button's label on its hover.
    ["--ink", "--button-fill", 4.5], ["--ink", "--button-hover", 4.5], ["--on-button-primary", "--button-primary-fill", 4.5],
    ["--on-button-primary", "--button-primary-hover", 4.5], ["--on-button-slate", "--button-slate-fill", 4.5], ["--on-button-slate", "--button-slate-hover", 4.5],
    ["--ink", "--segment-on", 4.5],
    // A stage icon's glyph is drawn in ink on its tint.
    ...tints.map((tint): [string, string, number] => ["--ink", tint, 4.5])
  ];
  const outlinePairs: Array<[string, string, number]> = [
    ["--slate", "--paper", 3], ["--slate", "--card", 3], ["--slate", "--fork-wash", 3], ["--outline-soft", "--card", 3],
    ["--slate-text", "--paper", 3], ["--slate-text", "--card", 3], ["--slate-text", "--fork-wash", 3], ["--slate-text", "--slate-wash", 3],
    // Upcoming tracks and the outline of a route that is not taken: diagram lines that must stay visible on their surfaces.
    ["--track-edge", "--paper", 3], ["--track-edge", "--card", 3], ["--sheet-focus", "--paper", 3], ["--control-edge", "--card", 3],
    // The travelled path against the upcoming rail it runs beside, and a button's edge against the card and the paper.
    ["--slate", "--track", 3], ["--button-edge", "--card", 2.5], ["--button-edge", "--paper", 2.5],
    // A checked checkbox's fill against an unchecked box.
    ["--control-on", "--card", 3],
    ...[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((line): [string, string, number] => [`--series-${line}`, "--paper", 3])
  ];

  it("keeps Dark's text at 4.5:1 or better on every surface it is drawn on", () => {
    const failing = textPairs.map(([fg, bg, min]) => [fg, bg, contrast(fg, bg, "dark"), min] as const).filter(([, , ratio, min]) => ratio < min);
    expect(failing).toEqual([]);
  });

  it("keeps Dark's focus, control and diagram outlines at 3:1 or better against their surfaces", () => {
    const failing = outlinePairs.map(([fg, bg, min]) => [fg, bg, contrast(fg, bg, "dark"), min] as const).filter(([, , ratio, min]) => ratio < min);
    expect(failing).toEqual([]);
  });

  it("raises Dark's selected segment above its track and keeps rails quieter than the travelled path", () => {
    expect(contrast("--segment-on", "--segment", "dark")).toBeGreaterThanOrEqual(1.8);
    // An upcoming rail is a quiet line, as Light's beige one is; the not-taken outline carries the 3:1.
    expect(contrast("--track", "--card", "dark")).toBeLessThan(2);
  });

  it("keeps the checkbox tick at 3:1 against a checked box's fill in both appearances", () => {
    // The tick is an SVG data URI in the checked rule, so its colour is written there rather than as a token.
    const fieldsCss = readFileSync(resolve(process.cwd(), "src/theme/fields.css"), "utf8");
    const tick = /input\[type="checkbox"\]:checked[\s\S]*?stroke='%23([0-9A-Fa-f]{6})'/.exec(fieldsCss)?.[1];
    expect(tick).toBeDefined();
    expect(contrast(`#${tick}`, "--control-on", "dark")).toBeGreaterThanOrEqual(3);
    expect(contrast(`#${tick}`, "--control-on", "light")).toBeGreaterThanOrEqual(3);
  });

  it("draws the needs badge's ! in on-accent ink, as the held badge already does", () => {
    const stagesCss = readFileSync(resolve(process.cwd(), "src/theme/stages/stages.css"), "utf8");
    expect(stagesCss).toMatch(/\.stage-box__dot \{[^}]*color: var\(--on-accent\)/);
    expect(stagesCss).toMatch(/\.stage-box__dot--needs \{ background: var\(--slate-text\);/);
  });

  it("keeps all eight local font files with their licences", () => {
    const webFonts = resolve(process.cwd(), "src/theme/fonts");
    for (const file of ["Archivo.woff2", "InterTight.woff2", "IBMPlexMono-Regular.woff2", "IBMPlexMono-Medium.woff2", "IBMPlexMono-SemiBold.woff2", "OFL-Archivo.txt", "OFL-IBMPlexMono.txt", "OFL-InterTight.txt"]) {
      expect(readFileSync(resolve(webFonts, file)).length, file).toBeGreaterThan(0);
    }
    expect(css.match(/@font-face\s*\{/g)).toHaveLength(5);
    expect(css).toContain('font-weight: 600;');
  });
});
