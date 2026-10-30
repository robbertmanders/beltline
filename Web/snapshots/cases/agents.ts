import { shellCases, sidebarState, type SnapshotCase } from "../caseTypes";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const library = '[data-column="agents-browser"]';
const editor = '[data-column="agents-editor"]';
const editorReady = '[data-column="agents-editor"][data-ready="true"]';

/**
 * Library browser, detail editor and assistant captures.
 * The assistant definitions cover native 6k-agents-agent and 6l-agents-agent-stopped.
 *
 * API fixtures establish repeatable daemon state before each viewport capture;
 * typed UI steps then exercise archived filters, drafts and reset confirmation.
 */
export const agentsCases: SnapshotCase[] = [
  ...shellCases("agents", "/agents"),
  {
    id: "agents-assistant-running",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true, assistantOpen: true }),
    readiness: "shell",
    apiResponses: [
      { path: "/api/v1/assistant", status: 200, body: JSON.stringify({
        ...JSON.parse(readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/assistant.json"), "utf8")),
        running: true,
        command: null,
        actions: [
          { id: "configuration", enabled: false, label: "Change configuration", reason: "Stop the assistant before changing its configuration." },
          { id: "start", enabled: false, label: "Start", reason: "The assistant is already running." },
          { id: "stop", enabled: true, label: "Stop", reason: null }
        ]
      }) },
      { path: "/api/v1/models/pi", status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/models.json"), "utf8") }
    ],
    expectedText: "Simulated conversation. No real agent is launched in demo mode.",
    readyMarker: '[data-column="assistant"]',
    visibleColumns: [library, '[data-column="assistant"]']
  },
  {
    id: "agents-assistant-running-terminal",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true, assistantOpen: true }),
    readiness: "shell",
    apiResponses: [
      { path: "/api/v1/assistant", status: 200, body: JSON.stringify({
        ...JSON.parse(readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/assistant.json"), "utf8")),
        running: true,
        command: ["pi"],
        actions: [
          { id: "configuration", enabled: false, label: "Change configuration", reason: "Stop the assistant before changing its configuration." },
          { id: "start", enabled: false, label: "Start", reason: "The assistant is already running." },
          { id: "stop", enabled: true, label: "Stop", reason: null }
        ]
      }) },
      { path: "/api/v1/models/pi", status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/models.json"), "utf8") }
    ],
    readyMarker: '[data-column="assistant"] .terminal-screen',
    visibleColumns: [library, '[data-column="assistant"]']
  },
  {
    id: "agents-assistant-stopped",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true, assistantOpen: true }),
    readiness: "shell",
    apiResponses: [
      { path: "/api/v1/assistant", status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/assistant.json"), "utf8") },
      { path: "/api/v1/models/pi", status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/models.json"), "utf8") }
    ],
    readyMarker: '[data-column="assistant"]',
    visibleColumns: [library, '[data-column="assistant"]']
  },
  {
    id: "agents-long-text",
    route: "/agents?agent=triage",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [
      { kind: "fill", target: { role: "textbox", name: "Brief" }, value: "A long brief for the Agents editor. This sentence checks that text wraps inside the editor column instead of widening the page. ".repeat(14) },
      { kind: "fill", target: { role: "textbox", name: "Outcome 1 when" }, value: "A long outcome description that remains readable and wraps within the outcome editor area. ".repeat(10) }
    ],
    readyMarker: '.agent-card:has(textarea[aria-label="Brief"])',
    visibleColumns: ['[data-column="agents-editor"]']
  },
  {
    id: "agents-empty",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "agents-empty" }],
    readyMarker: ".library-browser-empty",
    visibleColumns: [library]
  },
  {
    id: "agents-model-unavailable",
    route: "/agents?agent=triage",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "unavailable-model-choice", runner: "pi" }],
    readyMarker: '.model-picker[data-model-state="ready"]:has(.model-stale)',
    visibleColumns: ['[data-column="agents-editor"]']
  },
  {
    // The selected agent on Codex, with the thinking levels the demo daemon's Codex catalog offers.
    id: "agents-codex",
    route: "/agents?agent=triage",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "select", target: { role: "combobox", name: "Harness" }, value: "codex" }],
    readyMarker: '[data-column="agents-editor"][data-ready="true"]',
    visibleColumns: ['[data-column="agents-editor"]']
  },
  {
    // Two added models under one heading row, each with its own Harness, Model and Thinking and a Remove button.
    id: "agents-model-rows",
    route: "/agents?agent=triage",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [{ path: "/api/v1/models/pi", status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/models.json"), "utf8") }],
    steps: [
      { kind: "click", target: { role: "button", name: "+ Add a model" } },
      { kind: "click", target: { role: "button", name: "+ Add a model" } }
    ],
    // The last row's model picker is ready once the Pi catalog has loaded; the marker must match one element.
    readyMarker: `${editorReady} .agent-model-list > .agent-form-row:last-child .model-picker[data-model-state="ready"]`,
    visibleColumns: ['[data-column="agents-editor"]']
  },
  {
    id: "agents-cursor",
    route: "/agents?agent=triage",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "empty-model-choices", runner: "cursor" }],
    steps: [{ kind: "select", target: { role: "combobox", name: "Harness" }, value: "cursor" }],
    readyMarker: '.model-picker[data-model-state="empty"]',
    visibleColumns: ['[data-column="agents-editor"]']
  },
  {
    id: "agents-archived",
    route: "/agents?filter=archived",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "ensure-agent-archived", id: "review", archived: true }],
    readyMarker: ".library-browser-toolbar",
    visibleColumns: [library]
  },
  {
    id: "agents-revealed",
    route: "/agents?filter=archived",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "ensure-agent-archived", id: "review", archived: true }],
    readyMarker: '.library-resource-card[aria-label="Open Review"]',
    visibleColumns: [library]
  },
  {
    id: "agents-back-hidden",
    route: "/agents?filter=archived",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    fixtures: [{ kind: "ensure-agent-archived", id: "review", archived: true }],
    steps: [
      { kind: "click", target: { selector: '.library-resource-card[aria-label="Open Review"]' } },
      { kind: "click", target: { role: "button", name: "Back to Agents" } },
      { kind: "click", target: { role: "link", name: "Belts" } },
      { kind: "click", target: { role: "link", name: "Agents" } }
    ],
    readyMarker: ".library-browser",
    visibleColumns: [library]
  },
  {
    id: "agents-new",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "click", target: { role: "button", name: "New agent" } }],
    readyMarker: editorReady,
    visibleColumns: [editor]
  },
  {
    id: "agents-dirty",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [
      { kind: "click", target: { role: "button", name: "New agent" } },
      { kind: "wait", target: { role: "textbox", name: "Id" }, state: "visible" },
      { kind: "fill", target: { role: "textbox", name: "Name" }, value: "Buffer" }
    ],
    readyMarker: editorReady,
    visibleColumns: [editor]
  },
  {
    id: "agents-save-problems",
    route: "/agents",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [
      { kind: "click", target: { role: "button", name: "New agent" } },
      { kind: "wait", target: { role: "textbox", name: "Id" }, state: "visible" },
      { kind: "fill", target: { role: "textbox", name: "Name" }, value: " " },
      { kind: "click", target: { role: "button", name: "Save" } },
      { kind: "wait", target: { selector: ".agent-command-error" }, state: "visible" }
    ],
    readyMarker: ".agent-command-error",
    visibleColumns: [editor]
  },
  {
    id: "agents-used-by",
    route: "/agents?agent=verify",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "wait", target: { selector: ".agent-usage" }, state: "visible" }],
    readyMarker: ".agent-usage",
    visibleColumns: [editor]
  }
];
