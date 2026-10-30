import { readdirSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSnapshotSize, selectCases, snapshotCases, snapshotPageFiles, snapshotSizes } from "./cases";
import { validateSnapshotCase, type SnapshotCase } from "./caseTypes";
import { matchesRun, resolveRunSelector, type DemoRun } from "./runSelector";
import { beltsCases } from "./cases/belts";

const runCaseIds = [
  "run-prompt-entry", "run-open", "run-closed", "run-sessions-click", "run-wide", "run-wrapping-closed", "run-wrapping-open",
  "run-controls-open", "run-reduced-motion", "run-paused", "run-paused-sessions", "run-pause-incomplete",
  "run-waiting", "run-parked", "run-parked-sessions", "run-held", "run-review", "run-watch", "run-multi-watch",
  "run-nested", "run-afk-approved", "run-open-links", "run-stage-adjust", "run-send-to", "run-route"
];

describe("snapshot selection", () => {
  it("defines the page cases and three default sizes", () => {
    expect(snapshotCases.length).toBeGreaterThan(80);
    expect(snapshotSizes.map(({ width, height }) => `${width}x${height}`)).toEqual(["1080x680", "1440x900", "2560x1440"]);
  });
  it("covers every page and the Run and Agents interaction cases", () => {
    const ids = snapshotCases.map(item => item.id);
    const existing = ["overview", "work", "belts", "agents", "library", "afk", "settings"]
      .flatMap(name => [`${name}-open`, `${name}-closed`]);
    const interactive = ["run-sessions-click", "gallery", "agents-new", "agents-dirty", "agents-cursor", "agents-empty", "agents-long-text", "agents-model-unavailable",
      "agents-save-problems", "agents-used-by", "agents-archived", "agents-revealed", "agents-back-hidden",
      "agents-assistant-running", "agents-assistant-running-terminal", "agents-assistant-stopped", "agents-codex", "agents-model-rows"];
    const workOnly = ["work-epic", "work-grouped", "work-paused", "work-status", "work-watching"];
    const beltsOwn = beltsCases.map(item => item.id).filter(id => id !== "belts-open" && id !== "belts-closed");
    const overviewOwn = ["overview-floor-open", "overview-floor-closed", "overview-floor-pinned", "overview-paused"];
    const appearance = ids.filter(id => id.startsWith("appearance-"));
    const extra = [...appearance, ...runCaseIds, ...interactive, ...beltsOwn, ...overviewOwn, "library-shared", "library-compare", "afk-all", "settings-models-expanded", "native-7g-afk", "native-7i-afk-on", "native-7k-afk-decided", "native-7m-overrule-row", "native-7e-settings", "native-7g-settings-paused", "native-7j-settings-continued", "native-7f-settings-factories", "settings-model-error", "settings-inline-422", "library-share-belt", ...workOnly];
    expect(ids.filter(id => !extra.includes(id)).sort()).toEqual([...existing].sort());
    expect(interactive.every(id => ids.includes(id))).toBe(true);
    expect(ids.filter(id => workOnly.includes(id)).sort()).toEqual(workOnly);
    expect(ids).toEqual(expect.arrayContaining(beltsOwn));
    expect(ids).toContain("afk-all");
    expect(ids).toContain("settings-models-expanded");
    expect(ids).toContain("gallery");
    for (const id of runCaseIds) expect(ids).toContain(id);
  });
  it("keeps both navigation states of every Overview view", () => {
    const overview = selectCases("overview").map(item => `${item.id}:${item.sidebar?.navigationOpen}`);
    expect(overview).toEqual(expect.arrayContaining(["overview-open:true", "overview-closed:false", "overview-floor-open:true",
      "overview-floor-closed:false"]));
  });
  it("unions prefixes and accepts one custom size", () => {
    expect(selectCases("overview-open,gallery").map(item => item.id)).toEqual(["overview-open", "gallery"]);
    expect(selectCases("work")).toHaveLength(7);
    expect(selectCases("run").map(item => item.id)).toEqual(runCaseIds);
    expect(parseSnapshotSize("1280x720")).toEqual([{ width: 1280, height: 720 }]);
  });
  it.each(["0x900", "bad", "1x0"]) ("rejects malformed viewport %s", size => {
    expect(() => parseSnapshotSize(size)).toThrow(/positive WIDTHxHEIGHT/);
  });
  it("rejects unmatched and empty prefix lists", () => {
    expect(() => selectCases("does-not-exist")).toThrow(/matched no cases/);
    expect(() => selectCases(" , ")).toThrow(/matched no cases/);
  });
});

describe("Sessions layout cases", () => {
  it("keeps the Run cases that the layout assertions drive, with Sessions open or closed as their names say", () => {
    const layout = ["run-wrapping-closed", "run-wrapping-open", "run-sessions-click", "run-wide"];
    const byId = new Map(snapshotCases.map(item => [item.id, item] as const));
    for (const id of layout) expect(byId.has(id), `${id} is in the matrix`).toBe(true);
    expect(byId.get("run-wrapping-closed")!.sidebar!.sessionsOpen).toBe(false);
    expect(byId.get("run-wrapping-open")!.sidebar!.sessionsOpen).toBe(true);
    expect(byId.get("run-wide")!.sidebar!.sessionsOpen).toBe(true);
    expect(byId.get("run-sessions-click")!.readyMarker).toBe('[data-column="sessions"][data-ready="true"]');
  });
});

describe("assistant case definitions", () => {
  it("defines the Belts and Agents assistant states at the three default viewport sizes", () => {
    expect(snapshotSizes.map(({ width, height }) => `${width}x${height}`)).toEqual(["1080x680", "1440x900", "2560x1440"]);
    const required = ["belts-assistant-stopped", "belts-cursor-assistant", "belts-assistant-running-demo", "belts-assistant-stage-open",
      "agents-assistant-running", "agents-assistant-stopped"];
    const byId = new Map(snapshotCases.map(item => [item.id, item] as const));
    for (const caseId of required) {
      const item = byId.get(caseId);
      expect(item, `${caseId} has a case definition`).toBeDefined();
      expect(item!.sidebar?.assistantOpen).toBe(true);
      expect(item!.visibleColumns).toContain('[data-column="assistant"]');
      const pageColumns = caseId.startsWith("agents-")
        ? ['[data-column="agents-browser"]']
        : ['[data-column="belts-editor"]'];
      for (const column of pageColumns) expect(item!.visibleColumns).toContain(column);
      expect(item!.apiResponses?.some(response => response.path === "/api/v1/assistant")).toBe(true);
    }
    expect(byId.get("belts-assistant-stage-open")!.readyMarker).toBe('.stage-form[data-ready="true"]');
    expect(byId.get("belts-assistant-running-demo")!.expectedText).toContain("Simulated conversation.");
  });
});

describe("run selector resolution", () => {
  const runs: DemoRun[] = [
    { id: "issues:acme/platform#139", beltId: "issues", state: "waiting", issue: { number: 139, repo: "acme/platform", key: "" } },
    { id: "issues:acme/web#152", beltId: "issues", state: "running", issue: { number: 152, repo: "acme/web", key: "" } },
    { id: "epics:jira:PAY-40", beltId: "epics", state: "running", issue: { number: 0, repo: "acme/billing", key: "PAY-40" } }
  ];
  it("matches on every named field and ignores the rest", () => {
    expect(matchesRun(runs[1]!, { beltId: "issues", number: 152 })).toBe(true);
    expect(matchesRun(runs[1]!, { number: 152, repo: "acme/web" })).toBe(true);
    expect(matchesRun(runs[1]!, { state: "running", key: "" })).toBe(true);
    expect(matchesRun(runs[1]!, { beltId: "issues", number: 139 })).toBe(false);
    expect(matchesRun(runs[1]!, { repo: "acme/platform" })).toBe(false);
  });
  it("resolves the default and key-selected runs", () => {
    expect(resolveRunSelector("run-open", undefined, runs)).toBe("issues:acme/platform#139");
    expect(resolveRunSelector("run-watch", { beltId: "epics", key: "PAY-40" }, runs)).toBe("epics:jira:PAY-40");
  });
  it("fails by case id when no run matches", () => {
    expect(() => resolveRunSelector("run-missing", { beltId: "issues", number: 999 }, runs)).toThrow(/run-missing: no demo run matches/);
  });
});

describe("snapshot case files", () => {
  it("has unique ids and one registered file for every page case file on disk", () => {
    const ids = snapshotCases.map(item => item.id);
    expect(new Set(ids).size).toBe(ids.length);
    const onDisk = readdirSync(resolve(process.cwd(), "snapshots/cases")).filter(name => name.endsWith(".ts")).map(name => name.replace(/\.ts$/, "")).sort();
    expect(onDisk).toEqual([...snapshotPageFiles].sort());
  });
  it("uses booleans for every open sidebar", () => {
    for (const item of snapshotCases.filter(entry => entry.sidebar)) {
      expect(typeof item.sidebar!.navigationOpen).toBe("boolean");
      expect(typeof item.sidebar!.sessionsOpen).toBe("boolean");
      expect(typeof item.sidebar!.assistantOpen).toBe("boolean");
    }
  });
  it("starts the Sessions click case closed and expects it open after its steps", () => {
    const item = snapshotCases.find(entry => entry.id === "run-sessions-click")!;
    expect(item.sidebar!.sessionsOpen).toBe(false);
    expect(item.steps?.[0]).toEqual({ kind: "click", target: { role: "button", name: "Sessions", exact: true } });
    expect(item.expectedPreferences).toEqual({ sessionsOpen: true });
    expect(item.visibleColumns).toEqual(['[data-column="sessions"]']);
    expect(item.readyMarker).toBe('[data-column="sessions"][data-ready="true"]');
  });
  it("validates every registered case", () => {
    for (const item of snapshotCases) expect(() => validateSnapshotCase(item)).not.toThrow();
  });
});

describe("typed pre-capture steps", () => {
  const base: SnapshotCase = { id: "sample", route: "/work", sidebar: null, readiness: "shell" };
  it("accepts narrow error fixtures and rejects unrelated responses", () => {
    expect(() => validateSnapshotCase({ ...base, apiResponses: [{ path: "/api/v1/models/pi", status: 503, body: "{}" }], expectedText: "Catalog unavailable" })).not.toThrow();
    expect(() => validateSnapshotCase({ ...base, apiResponses: [{ path: "/auth", status: 200, body: "{}" }] })).toThrow(/API snapshot overrides/);
  });
  it("accepts click, fill, select, press, set-button and wait steps", () => {
    expect(() => validateSnapshotCase({
      ...base,
      steps: [
        { kind: "click", target: { role: "button", name: "Sessions" } },
        { kind: "set-button", target: { role: "button", name: "Pause" }, opposite: { role: "button", name: "Resume" } },
        { kind: "fill", target: { role: "textbox", name: "Message" }, value: "hello" },
        { kind: "select", target: { role: "combobox", name: "Harness" }, value: "cursor" },
        { kind: "press", target: { role: "textbox", name: "Message" }, key: "Tab" },
        { kind: "wait", target: { selector: "[data-ready]" }, state: "visible" }
      ],
      readyMarker: "[data-ready]"
    })).not.toThrow();
  });
  it("rejects unknown kinds, missing targets, empty names and bad wait states", () => {
    const invalid = (step: unknown) => () => validateSnapshotCase({ ...base, steps: [step as never], readyMarker: "x" });
    expect(invalid({ kind: "script", target: { selector: "x" } })).toThrow(/unknown step kind/);
    expect(invalid({ kind: "click" })).toThrow(/needs a target/);
    expect(invalid({ kind: "click", target: { role: "button", name: " " } })).toThrow(/accessible role and name/);
    expect(invalid({ kind: "wait", target: { selector: "x" }, state: "later" })).toThrow(/visible or hidden/);
    expect(invalid({ kind: "set-button", target: { role: "button", name: "Pause" } })).toThrow(/opposite button name/);
    expect(invalid({ kind: "fill", target: { selector: "x" }, value: 3 })).toThrow(/string value/);
  });
  it("requires a readiness marker whenever a case has steps", () => {
    expect(() => validateSnapshotCase({ ...base, steps: [{ kind: "click", target: { selector: "button" } }] })).toThrow(/needs a readyMarker/);
  });
  it("rejects non-boolean sidebars and ids that are not stable slugs", () => {
    expect(() => validateSnapshotCase({ ...base, sidebar: { navigationOpen: "yes", navigationWidth: 220, sessionsOpen: false, assistantOpen: false } as never })).toThrow(/must be a boolean/);
    expect(() => validateSnapshotCase({ ...base, id: "Bad Id" })).toThrow(/lowercase words/);
  });
  it("requires a valid run selector with a route function", () => {
    const runRoute = (runId: string) => `/runs/${runId}`;
    expect(() => validateSnapshotCase({ ...base, id: "run-sample", route: runRoute, run: {} })).toThrow(/at least one field/);
    expect(() => validateSnapshotCase({ ...base, id: "run-sample", route: runRoute, run: { number: 0 } })).toThrow(/positive integer/);
    expect(() => validateSnapshotCase({ ...base, id: "run-sample", route: runRoute, run: { beltId: " " } })).toThrow(/non-empty string/);
    expect(() => validateSnapshotCase({ ...base, id: "run-sample", run: { beltId: "issues" } })).toThrow(/needs a route function/);
    expect(() => validateSnapshotCase({ ...base, id: "run-sample", route: runRoute, run: { beltId: "issues", number: 139 } })).not.toThrow();
  });
});
