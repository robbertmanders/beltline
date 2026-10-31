import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { AttemptOutputSection, MarkdownBlock, RunViewResponse } from "../../src/api/wire.gen";
import { sidebarState, type SnapshotCase, type RunSelector } from "../caseTypes";

const sessionsPanel = '[data-column="sessions"]';
const sessionsReady = '[data-column="sessions"][data-ready="true"]';

const issues = (number: number): RunSelector => ({ beltId: "issues", number });
const runRoute = (runId: string) => `/runs/${encodeURIComponent(runId)}`;
const stageRoute = (runId: string, stage: string, attempt?: string) =>
  `/runs/${encodeURIComponent(runId)}?stage=${encodeURIComponent(stage)}${attempt ? `&attempt=${encodeURIComponent(attempt)}` : ""}`;

function promptEntryResponse(): string {
  const view = JSON.parse(readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/views-run.json"), "utf8")) as RunViewResponse;
  const panel = view.stages.find(stage => stage.id === "approve-plan")!;
  view.run.state = "waiting";
  view.run.stageId = panel.id;
  panel.name = "Prompt";
  panel.step = "waiting";
  panel.attempts = [{ id: "approve-plan-1", number: 1, caption: "Attempt 1 of 1", content: "decision", isOpen: true }];
  const facts = panel.facts!;
  facts.state = { text: "WAITING FOR YOU", tone: "you" };
  facts.kindChip = "YOUR PROMPT";
  facts.attempts = [];
  const approval = facts.approval!;
  delete approval.blocks;
  approval.primary = "started";
  approval.outcomes = [
    { id: "started", name: "started", label: "Start", tone: "good" },
    { id: "parked", name: "parked", label: "Park it", tone: "neutral" }
  ];
  approval.prompt = {
    label: "Your prompt",
    placeholder: "Describe what you want the agents to do, with any constraints and acceptance criteria.",
    help: "Write the run's task, then choose how to go on. Every stage after this one gets it.",
    contextLabel: "Source context",
    context: [
      { type: "heading", level: 2, text: "The Jira ticket" },
      { type: "paragraph", text: "PAY-40: Split invoicing into its own service" },
      { type: "paragraph", text: "Extract invoicing behind a stable API. Preserve existing invoice identifiers and make the migration reversible." }
    ],
    sourceLabel: "Include source context for agents",
    sourceHelp: "Unchecked, agents get your prompt and the work item's reference only, not its title, description, labels, parent or link. Branch names and folders still carry the reference.",
    source: "withheld"
  };
  view.actions = approval.outcomes.map(outcome => ({
    id: "decide", enabled: true, stage: panel.id, outcome: outcome.name, needsNote: outcome.id === "started"
  }));
  return JSON.stringify(view);
}

const outputSectionsRun = "epics:jira:PAY-40";
const outputSectionsAttempt = "plan-epic-1";
const outputSectionsRoute = `/runs/${encodeURIComponent(outputSectionsRun)}?stage=plan-epic&attempt=${outputSectionsAttempt}`;
const outputSectionsParagraph = (text: string): MarkdownBlock => ({ type: "paragraph", text });

/** The output's sections, as the daemon presents them: the preamble, then each heading with its body. */
const outputSectionsSections = (): AttemptOutputSection[] => [
  { id: "preamble", label: "Output", level: 0, blocks: [outputSectionsParagraph("This plan splits invoicing out behind one stable API, in three waves.")] },
  { id: "heading:2:1:Goal", label: "Goal", level: 2, blocks: [outputSectionsParagraph("Move **invoice issuing** into its own service and keep existing callers working.")] },
  { id: "heading:2:1:Approach", label: "Approach", level: 2, blocks: [
    { type: "item", marker: "•", text: "Extract the API behind a facade, with the current identifiers unchanged." },
    { type: "item", marker: "•", text: "Add contract tests for every endpoint the callers use." },
    { type: "item", marker: "1.", text: "Migrate the data after the callers have moved, behind a reversible step." }
  ] },
  { id: "heading:3:1:A very long section heading that keeps going so the label has to wrap inside the stage panel at the minimum supported width",
    label: "A very long section heading that keeps going so the label has to wrap inside the stage panel at the minimum supported width", level: 3,
    blocks: [outputSectionsParagraph("A long paragraph under the long heading, so the expanded body is checked against the same column as the label above it.")] },
  { id: "heading:2:1:Testing", label: "Testing", level: 2, blocks: [{ type: "code", text: "$ invoice-service contract --endpoint /v1/invoices --expect-identifiers-unchanged --region eu-west-1 --timeout 30s --report artifacts/contract-report-with-a-long-file-name.json" }] },
  { id: "heading:2:1:Open questions", label: "Open questions", level: 2, blocks: [{ type: "item", marker: "•", text: "Should the migration run before or after the first wave ships?" }] }
];

/** The flat block list the daemon sends beside the sections: the title, every heading and every body in order. */
function outputSectionsBlocks(title: MarkdownBlock, sections: AttemptOutputSection[]): MarkdownBlock[] {
  return [title, ...sections.flatMap(section => section.level > 0 ? [{ type: "heading" as const, level: section.level, text: section.label }, ...section.blocks] : section.blocks)];
}

function outputSectionsCases(): SnapshotCase[] {
  const title: MarkdownBlock = { type: "heading", level: 1, text: "Plan: split invoicing into its own service" };
  const sections = outputSectionsSections();
  const output = JSON.stringify({ runId: outputSectionsRun, attemptId: outputSectionsAttempt, availability: "available", blocks: outputSectionsBlocks(title, sections), title, sections });
  const view = readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/views-run.json"), "utf8");
  const apiResponses = [
    { path: `/api/v1/views/run/${encodeURIComponent(outputSectionsRun)}`, status: 200, body: view },
    { path: `/api/v1/runs/${encodeURIComponent(outputSectionsRun)}/attempts/${outputSectionsAttempt}/output`, status: 200, body: output },
    { path: `/api/v1/runs/${encodeURIComponent(outputSectionsRun)}/attempts/${outputSectionsAttempt}/artifacts`, status: 200, body: readFileSync(resolve(process.cwd(), "../daemon/testdata/golden/run-attempt-artifacts.json"), "utf8") }
  ];
  return [
    {
      // Every section collapsed in Light: the title and the section labels, nothing else.
      id: "run-output-sections-collapsed",
      route: outputSectionsRoute,
      sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
      readiness: "run",
      apiResponses,
      readyMarker: ".run-output-sections",
      scrollToTop: ".run-output-sections"
    },
    {
      // The Testing section open in Dark, through a typed click on its control.
      id: "run-output-sections-dark-expanded",
      route: outputSectionsRoute,
      sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
      readiness: "run",
      appearance: "dark",
      apiResponses,
      steps: [{ kind: "click", target: { role: "button", name: "Testing", exact: true } }],
      readyMarker: ".output-section.is-open",
      scrollToTop: ".run-output-sections"
    }
  ];
}

/**
 * The Run page draws the daemon-provided belt as stage boxes, its stage panel, output and artifacts, so its cases wait on
 * the Run content itself rather than the old resource scaffold. The default demo run is the seeded
 * `acme/platform#139` run; a case can name a different seeded run with `run` and the harness resolves
 * it from `/api/v1/runs`. Its last saved agent output is `spec-1`, which the demo seeds with long
 * Markdown when the snapshot daemon starts.
 */
export const runCases: SnapshotCase[] = [
  {
    id: "run-prompt-entry",
    route: "/runs/epics%3Ajira%3APAY-40?stage=approve-plan",
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    apiResponses: [{ path: "/api/v1/views/run/epics%3Ajira%3APAY-40", status: 200, body: promptEntryResponse() }],
    readyMarker: ".run-prompt textarea",
    scrollToTop: ".stage-panel",
    steps: [{ kind: "fill", target: { role: "textbox", name: "Your prompt", exact: true },
      value: "Prepare the API boundary for invoicing. Keep existing callers working and add focused contract tests.\n\nDo not perform the data migration yet." }]
  },
  { id: "run-open", route: runRoute, sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }), readiness: "run" },
  { id: "run-closed", route: runRoute, sidebar: sidebarState({ navigationOpen: false, sessionsOpen: false }), readiness: "run" },
  {
    // A new snapshot starts with Sessions closed, clicks the real button, and captures the panel once it is ready.
    id: "run-sessions-click",
    route: runRoute,
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    steps: [
      { kind: "click", target: { role: "button", name: "Sessions", exact: true } },
      { kind: "wait", target: { selector: sessionsReady }, state: "visible" }
    ],
    readyMarker: sessionsReady,
    expectedPreferences: { sessionsOpen: true },
    visibleColumns: [sessionsPanel]
  },
  {
    id: "run-wide",
    route: runRoute,
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: true }),
    readiness: "run",
    readyMarker: sessionsReady
  },
  {
    // The long Markdown fixture, with Sessions closed. capture.spec.ts also runs it, and the open case below,
    // at 1080×680, 1440×900 and 2560×1440 for overflow assertions without a screenshot.
    id: "run-wrapping-closed",
    route: (runId: string) => stageRoute(runId, "spec", "spec-1"),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    id: "run-wrapping-open",
    route: (runId: string) => stageRoute(runId, "spec", "spec-1"),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: true }),
    readiness: "run",
    readyMarker: sessionsReady
  },
  // Agent output as folding sections: several headings, a long heading and long code. The run view and the
  // attempt's output and artifacts are fixed here, so the captures do not depend on the demo's saved output.
  ...outputSectionsCases(),
  {
    // The floating run menu (controls, then links), so its bounds are visible next to the page it floats over.
    id: "run-controls-open",
    route: runRoute,
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    steps: [
      { kind: "click", target: { role: "button", name: "Run actions", exact: true } },
      { kind: "wait", target: { selector: ".run-menu-card" }, state: "visible" }
    ],
    readyMarker: ".run-menu-card"
  },
  {
    id: "run-reduced-motion",
    route: runRoute,
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    reducedMotion: "reduce"
  },
  // Distinct native Run states. Each names its seeded demo run so the capture fails rather than
  // silently replacing a fixture.
  {
    // 4p-run-paused: the paused mid-triage run #152.
    id: "run-paused",
    route: runRoute,
    run: issues(152),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4p-run-paused-sessions: the same paused run with the Sessions panel open.
    id: "run-paused-sessions",
    route: runRoute,
    run: issues(152),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: true }),
    readiness: "run",
    readyMarker: sessionsReady
  },
  {
    // 4p-pause-incomplete: the security run #93 whose pause did not confirm.
    id: "run-pause-incomplete",
    route: runRoute,
    run: { beltId: "security", number: 93 },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4b-run-waiting: the waiting run #88 (the pause seed's waiting state).
    id: "run-waiting",
    route: runRoute,
    run: { beltId: "security", number: 88 },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4b5-run-parked / 4b6-run-parked-sessions: the demo's parked run #153. The native cases park a
    // waiting run through the coordinator; the browser capture reads the seeded parked run instead.
    id: "run-parked",
    route: runRoute,
    run: issues(153),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    id: "run-parked-sessions",
    route: runRoute,
    run: issues(153),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: true }),
    readiness: "run",
    readyMarker: sessionsReady
  },
  {
    // 5b-run-held: the held run #151.
    id: "run-held",
    route: runRoute,
    run: issues(151),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4c-run-review: the pull-request review run #160.
    id: "run-review",
    route: runRoute,
    run: { beltId: "reviews", number: 160 },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4d-run-watch and 4d3-run-epic both resolve to the epic's ticket watch (PAY-40), the first
    // running watch in the daemon's createdAt order.
    id: "run-watch",
    route: runRoute,
    run: { beltId: "epics", key: "PAY-40" },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4d2-run-multi-watch: the jira run PAY-34, whose watch shows two pull requests.
    id: "run-multi-watch",
    route: runRoute,
    run: { beltId: "jira", key: "PAY-34" },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4n-nested-run: the demo's run #180 on the belt whose route splits again.
    id: "run-nested",
    route: runRoute,
    run: { beltId: "nested", number: 180 },
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 7h-afk-run: run #142 at Approve, the stage the demo's AFK decision approved.
    id: "run-afk-approved",
    route: (runId: string) => stageRoute(runId, "approve"),
    run: issues(142),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  },
  {
    // 4o-open-links: the run menu of the default run, opened at its links section with the ticket and worktree entries.
    id: "run-open-links",
    route: runRoute,
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    steps: [
      { kind: "click", target: { role: "button", name: "Run actions", exact: true } },
      { kind: "wait", target: { selector: ".run-menu-card" }, state: "visible" }
    ],
    readyMarker: ".run-menu-card"
  },
  {
    // 5c-stage-adjust: the Adjust stage sheet for the default run's Spec stage, opened from the stage panel's Adjust.
    id: "run-stage-adjust",
    route: (runId: string) => stageRoute(runId, "spec", "spec-1"),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    steps: [
      { kind: "click", target: { role: "button", name: "Adjust", exact: true } },
      { kind: "wait", target: { selector: '[role="dialog"]' }, state: "visible" }
    ],
    readyMarker: '[role="dialog"]'
  },
  {
    // 5e-send-to: the Send to… sheet of the held run #151, opened from the run menu.
    id: "run-send-to",
    route: runRoute,
    run: issues(151),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run",
    steps: [
      { kind: "click", target: { role: "button", name: "Run actions", exact: true } },
      { kind: "click", target: { role: "button", name: "Send to…", exact: true } },
      { kind: "wait", target: { selector: '[role="dialog"]' }, state: "visible" }
    ],
    readyMarker: '[role="dialog"]'
  },
  {
    // 4e-run-route: the run #145 the demo estimates on a split route.
    id: "run-route",
    route: runRoute,
    run: issues(145),
    sidebar: sidebarState({ navigationOpen: true, sessionsOpen: false }),
    readiness: "run"
  }
];