import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(resolve(process.cwd(), "src/theme/tokens.css"), "utf8");

/** The declarations of every rule with exactly this selector, merged. Light is `:root`; Dark is the `data-theme` attribute's rule. */
function ruleDeclarations(selector: string): Record<string, string> {
  const merged: Record<string, string> = {};
  for (let start = css.indexOf(`\n${selector} {`); start >= 0; start = css.indexOf(`\n${selector} {`, start + 1)) {
    const body = css.slice(start + selector.length + 3, css.indexOf("}", start));
    for (const match of body.matchAll(/(--[\w-]+):\s*([^;]+);/g)) merged[match[1]!] = match[2]!.trim();
  }
  if (!Object.keys(merged).length) throw new Error(`Missing rule ${selector}`);
  return merged;
}

const declarations = { ...ruleDeclarations(":root"), ...ruleDeclarations(":root, .inv") };
const darkDeclarations = ruleDeclarations(':root[data-theme="dark"]');
// The roles are mixed from the pair in one rule; Light's own rule is the first `:root {`, which also holds them.
const roles = Object.keys(declarations).filter(name => !["--A", "--B", "--P", "--I"].includes(name) && /^(var|color-mix|#)/.test(declarations[name]!));
const tints = Object.keys(declarations).filter(name => name.startsWith("--tint-"));
const isLiteralColour = (value: string) => /^#|^rgba?\(/.test(value);

type Colour = { r: number; g: number; b: number; a: number };

function parseColour(value: string): Colour {
  const hex = /^#([0-9a-f]{3,8})$/i.exec(value);
  if (hex) {
    const digits = hex[1]!.length <= 4 ? [...hex[1]!].map(digit => digit + digit).join("") : hex[1]!;
    const channel = (at: number) => parseInt(digits.slice(at, at + 2), 16);
    return { r: channel(0), g: channel(2), b: channel(4), a: digits.length === 8 ? channel(6) / 255 : 1 };
  }
  throw new Error(`Not a literal colour: ${value}`);
}

/** The colour a token takes in a theme: Dark only redeclares the pair, so every role follows it. Handles var() and color-mix in srgb. */
function resolveToken(name: string, theme: "light" | "dark"): Colour {
  const value = (theme === "dark" && darkDeclarations[name]) || declarations[name];
  if (!value) throw new Error(`Missing token ${name}`);
  return resolveValue(value, theme);
}

function resolveValue(value: string, theme: "light" | "dark"): Colour {
  if (value === "transparent") return { r: 0, g: 0, b: 0, a: 0 };
  const alias = /^var\((--[\w-]+)\)$/.exec(value);
  if (alias) return resolveToken(alias[1]!, theme);
  const mix = /^color-mix\(in srgb, (.+?) (\d+(?:\.\d+)?)%, (.+)\)$/.exec(value);
  if (mix) {
    const first = resolveValue(mix[1]!, theme), second = resolveValue(mix[3]!, theme), share = Number(mix[2]) / 100;
    // A mix with transparent keeps the colour and scales its alpha, as the browser's premultiplied mix does.
    if (second.a === 0) return { ...first, a: first.a * share };
    const blend = (x: number, y: number) => x * share + y * (1 - share);
    return { r: blend(first.r, second.r), g: blend(first.g, second.g), b: blend(first.b, second.b), a: blend(first.a, second.a) };
  }
  return parseColour(value);
}

function composite(top: Colour, below: Colour): Colour {
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

/** WCAG contrast of a foreground token against a background token in one theme, compositing translucent colours over the paper. */
function contrast(foreground: string, background: string, theme: "light" | "dark"): number {
  const surface = composite(resolveToken(background, theme), resolveToken("--paper", theme));
  const ink = composite(resolveToken(foreground, theme), surface);
  const [light, dark] = [luminance(ink), luminance(surface)].sort((a, b) => b - a) as [number, number];
  return (light + 0.05) / (dark + 0.05);
}

describe("Two Inks tokens", () => {
  it("keeps Light as the default and Dark only behind the appearance attribute", () => {
    expect(css).toContain(":root {\n  color-scheme: light;");
    expect(css).toContain(':root[data-theme="dark"] {\n  color-scheme: dark;');
    expect(css).not.toMatch(/prefers-color-scheme\s*:\s*dark/);
    expect(declarations["--font-size-body"]).toBe("14px");
    expect(declarations["--hairline-width"]).toBe("1px");
  });

  it("draws every colour from one paper and one ink", () => {
    expect(declarations["--A"]).toBe("#ECE4DF");
    expect(declarations["--B"]).toBe("#420C12");
    expect(darkDeclarations["--A"]).toBe("#22070A");
    expect(darkDeclarations["--B"]).toBe("#EFE2DC");
    // Dark swaps the pair and nothing else, so it cannot drift from Light.
    expect(Object.keys(darkDeclarations).sort()).toEqual(["--A", "--B"]);
    // A part drawn the other way round swaps the paper and the ink, and darkens the screen it sits on.
    const inverse = ruleDeclarations(".inv");
    expect(inverse["--P"]).toBe("var(--B)");
    expect(inverse["--I"]).toBe("var(--A)");
    // The only literal colours are the pair and the screen, which is the light ink in both appearances.
    const literal = Object.keys(declarations).filter(name => isLiteralColour(declarations[name]!));
    expect(literal.sort()).toEqual(["--A", "--B", "--screen", "--screen-bar", "--screen-dim", "--screen-text"]);
    expect(roles.length).toBeGreaterThan(100);
    for (const name of roles) resolveToken(name, "dark");
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
    ["--on-accent", "--segment-on", 4.5],
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

  for (const theme of ["light", "dark"] as const) {
    it(`keeps ${theme} text at 4.5:1 or better on every surface it is drawn on`, () => {
      const failing = textPairs.map(([fg, bg, min]) => [fg, bg, contrast(fg, bg, theme).toFixed(2), min] as const).filter(([, , ratio, min]) => Number(ratio) < min);
      expect(failing).toEqual([]);
    });

    it(`keeps ${theme} focus, control and diagram outlines at 3:1 or better against their surfaces`, () => {
      const failing = outlinePairs.map(([fg, bg, min]) => [fg, bg, contrast(fg, bg, theme).toFixed(2), min] as const).filter(([, , ratio, min]) => Number(ratio) < min);
      expect(failing).toEqual([]);
    });

    it(`keeps ${theme} rails quieter than the travelled path`, () => {
      expect(contrast("--track", "--card", theme)).toBeLessThan(2);
      expect(contrast("--slate", "--card", theme)).toBeGreaterThan(contrast("--track", "--card", theme));
    });

    it(`keeps the terminal screen readable in ${theme}`, () => {
      expect(contrast("--screen-text", "--screen", theme)).toBeGreaterThanOrEqual(7);
      expect(contrast("--screen-dim", "--screen", theme)).toBeGreaterThanOrEqual(4.5);
    });
  }

  it("draws a checked checkbox as the ink inside a ring of paper", () => {
    const fieldsCss = readFileSync(resolve(process.cwd(), "src/theme/fields.css"), "utf8");
    expect(fieldsCss).toMatch(/input\[type="checkbox"\]:checked \{[^}]*background-color: var\(--control-on\);[^}]*box-shadow: inset 0 0 0 [\d.]+px var\(--paper\)/);
  });

  it("draws the needs badge's ! in on-accent ink, as the held badge already does", () => {
    const stagesCss = readFileSync(resolve(process.cwd(), "src/theme/stages/stages.css"), "utf8");
    expect(stagesCss).toMatch(/\.stage-box__dot \{[^}]*color: var\(--on-accent\)/);
    expect(stagesCss).toMatch(/\.stage-box__dot--needs \{ background: var\(--slate-text\);/);
  });

  it("keeps the three local font families, each in two subsets, with their licences", () => {
    const webFonts = resolve(process.cwd(), "src/theme/fonts");
    for (const file of ["FunnelDisplay-Latin.woff2", "FunnelDisplay-LatinExt.woff2", "FunnelSans-Latin.woff2", "FunnelSans-LatinExt.woff2",
      "GeistMono-Latin.woff2", "GeistMono-LatinExt.woff2", "OFL-FunnelDisplay.txt", "OFL-FunnelSans.txt", "OFL-GeistMono.txt"]) {
      expect(readFileSync(resolve(webFonts, file)).length, file).toBeGreaterThan(0);
    }
    expect(css.match(/@font-face\s*\{/g)).toHaveLength(6);
    expect(declarations["--font-display"]).toBe('"Funnel Display"');
    expect(declarations["--font-body"]).toBe('"Funnel Sans"');
    expect(declarations["--font-mono"]).toBe('"Geist Mono"');
  });
});
