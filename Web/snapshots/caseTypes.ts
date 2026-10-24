import type { RunSelector } from "./runSelector";

export type { RunSelector } from "./runSelector";

// Typed snapshot case descriptions. A case is data: the capture harness runs its steps in order and
// nothing else. Arbitrary page scripts are deliberately not part of this vocabulary.

export interface SidebarState {
  navigationOpen: boolean;
  navigationWidth: number;
  sessionsOpen: boolean;
  assistantOpen: boolean;
}

/** An element found the way a person finds it: by role and accessible name. */
export interface AccessibleTarget {
  role: "button" | "link" | "checkbox" | "textbox" | "combobox";
  name: string;
  exact?: boolean;
}

/** A CSS selector, for elements that have no accessible name of their own, such as readiness markers. */
export interface SelectorTarget {
  selector: string;
}

export type StepTarget = AccessibleTarget | SelectorTarget;

export type PreCaptureStep =
  | { kind: "click"; target: StepTarget }
  | { kind: "set-button"; target: AccessibleTarget; opposite: AccessibleTarget }
  | { kind: "fill"; target: StepTarget; value: string }
  | { kind: "select"; target: StepTarget; value: string }
  | { kind: "press"; target: StepTarget; key: string }
  | { kind: "wait"; target: StepTarget; state: "visible" | "hidden" };

/** Idempotent API fixtures applied before the page loads, so mutations do not leak between viewport captures. */
export type SnapshotFixture =
  | { kind: "ensure-agent-archived"; id: string; archived: boolean }
  | { kind: "ensure-agent-reset-available"; id: string }
  | { kind: "agents-empty" }
  | { kind: "empty-model-choices"; runner: string }
  | { kind: "unavailable-model-choice"; runner: string };

/** One page capture: where to go, how the shell is arranged, and any steps before the screenshot. */
export interface SnapshotCase {
  id: string;
  route: string | ((runId: string) => string);
  /** The demo run a route-function case draws. Defaults to the seeded acme/platform#139 run. */
  run?: RunSelector;
  sidebar: SidebarState | null;
  readiness: "shell" | "gallery" | "run";
  /** Run after base page readiness, in order. A missing target fails the capture. */
  steps?: readonly PreCaptureStep[];
  /** Establish deterministic daemon state before navigation. */
  fixtures?: readonly SnapshotFixture[];
  /** Must match exactly one visible element once the steps finish. */
  readyMarker?: string;
  /** Emulate the browser's reduced-motion preference for this capture. */
  reducedMotion?: "reduce" | "no-preference";
  /** The appearance the browser stores before the page loads, so the capture starts in that appearance. */
  appearance?: "light" | "dark";
  /** Browser preference values the capture must observe after the steps. */
  expectedPreferences?: Partial<SidebarState>;
  /** Test-only browser API responses used to reach stable error states. A `query` must equal the request's query string; without one any query matches. */
  apiResponses?: readonly { path: string; status: number; body: string; query?: string }[];
  /** A visible message required after pre-capture steps have completed. */
  expectedText?: string;
  /** Selectors of columns that must be visible with a nonzero width when captured. */
  visibleColumns?: readonly string[];
  /** Scroll the belt editor so a stage form starts at the top of the capture, for a stage whose line is taller than the window. */
  formAtTop?: boolean;
  /** A CSS selector for a section below the fold: it is scrolled to the top of the capture once the steps finish. */
  scrollToTop?: string;
}

export function sidebarState(changes: Partial<SidebarState> = {}): SidebarState {
  return { navigationOpen: true, navigationWidth: 220, sessionsOpen: false, assistantOpen: false, ...changes };
}

/** The open and closed navigation pair every shell page is captured in. */
export function shellCases(name: string, route: SnapshotCase["route"]): SnapshotCase[] {
  return [
    { id: `${name}-open`, route, sidebar: sidebarState({ navigationOpen: true }), readiness: "shell" },
    { id: `${name}-closed`, route, sidebar: sidebarState({ navigationOpen: false }), readiness: "shell" }
  ];
}

export function describeTarget(target: StepTarget): string {
  return "selector" in target ? `selector ${target.selector}` : `${target.role} "${target.name}"`;
}

const stepKinds = ["click", "set-button", "fill", "select", "press", "wait"] as const;

/** Rejects a case that the harness could not run; called for every registered case. */
export function validateSnapshotCase(item: SnapshotCase): void {
  function fail(message: string): never {
    throw new Error(`snapshot case ${item.id}: ${message}`);
  }
  if (!/^[a-z][a-z0-9-]*$/.test(item.id)) fail("the id must be lowercase words joined by hyphens");
  if (item.appearance !== undefined && item.appearance !== "light" && item.appearance !== "dark") fail("appearance must be light or dark");
  if (item.sidebar) {
    for (const key of ["navigationOpen", "sessionsOpen", "assistantOpen"] as const) {
      if (typeof item.sidebar[key] !== "boolean") fail(`${key} must be a boolean`);
    }
    if (!Number.isInteger(item.sidebar.navigationWidth) || item.sidebar.navigationWidth <= 0) fail("navigationWidth must be a positive integer");
  }
  for (const step of item.steps ?? []) {
    if (!stepKinds.includes(step.kind)) fail(`unknown step kind ${JSON.stringify((step as { kind: unknown }).kind)}`);
    const target = step.target as StepTarget | undefined;
    if (!target || typeof target !== "object") fail(`${step.kind} step needs a target`);
    if ("selector" in target) {
      if (!target.selector.trim()) fail("a selector target must not be empty");
    } else if (!("role" in target) || typeof target.name !== "string" || !target.name.trim()) {
      fail(`${step.kind} step needs an accessible role and name`);
    }
    if ((step.kind === "fill" || step.kind === "select") && typeof step.value !== "string") fail(`${step.kind} needs a string value`);
    if (step.kind === "press" && (typeof step.key !== "string" || !step.key.trim())) fail("press needs a key");
    if (step.kind === "set-button" && (!step.opposite || step.opposite.role !== "button" || !step.opposite.name.trim())) fail("set-button needs an opposite button name");
    if (step.kind === "wait" && step.state !== "visible" && step.state !== "hidden") fail("wait needs a visible or hidden state");
  }
  for (const fixture of item.fixtures ?? []) {
    if (!["ensure-agent-archived", "ensure-agent-reset-available", "agents-empty", "empty-model-choices", "unavailable-model-choice"].includes(fixture.kind)) fail("unknown fixture kind");
    if (fixture.kind === "agents-empty") continue;
    if (fixture.kind === "empty-model-choices" || fixture.kind === "unavailable-model-choice") {
      if (typeof fixture.runner !== "string" || !fixture.runner.trim()) fail("model fixture needs a runner");
      continue;
    }
    if (typeof fixture.id !== "string" || !fixture.id.trim()) fail("agent fixture needs an id");
    if (fixture.kind === "ensure-agent-archived" && typeof fixture.archived !== "boolean") fail("archive fixture needs a boolean state");
  }
  if ((item.steps?.length ?? 0) > 0 && !item.readyMarker) fail("a case with steps needs a readyMarker");
  if (item.visibleColumns?.some(selector => !selector.trim())) fail("visibleColumns must not be empty");
  if (item.scrollToTop !== undefined && !item.scrollToTop.trim()) fail("scrollToTop must be a selector");
  if (item.run) {
    const fields = [item.run.beltId, item.run.repo, item.run.key, item.run.state, item.run.number];
    if (fields.every(value => value === undefined)) fail("a run selector needs at least one field");
    for (const field of ["beltId", "repo", "key", "state"] as const) {
      if (item.run[field] !== undefined && (typeof item.run[field] !== "string" || !item.run[field]!.trim())) fail(`run selector ${field} must be a non-empty string`);
    }
    if (item.run.number !== undefined && (!Number.isInteger(item.run.number) || item.run.number <= 0)) fail("run selector number must be a positive integer");
    if (typeof item.route !== "function") fail("a run selector needs a route function");
  }
  if (item.apiResponses?.some(response => !response.path.startsWith("/api/") || response.status < 200 || response.status > 599 || !response.body)) fail("API snapshot overrides need an /api/ path, valid status and body");
}
