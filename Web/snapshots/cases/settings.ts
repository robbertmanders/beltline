import { shellCases, sidebarState, type SnapshotCase } from "../caseTypes";
import { snapshotState } from "../stateFixture";

export const settingsCases: SnapshotCase[] = [
  ...shellCases("settings", "/settings"),
  { id: "native-7e-settings", route: "/settings", sidebar: sidebarState({ navigationOpen: true }), readiness: "shell", readyMarker: 'section[aria-label="General"]' },
  { id: "native-7g-settings-paused", route: "/settings", sidebar: sidebarState({ navigationOpen: true }), readiness: "shell", apiResponses: [{ path: "/api/v1/state", status: 200, body: snapshotState({ paused: true }) }], steps: [{ kind: "wait", target: { selector: '.global-controls button[aria-label^="Resume"]' }, state: "visible" }], readyMarker: 'section[aria-label="General"]' },
  { id: "native-7j-settings-continued", route: "/settings", sidebar: sidebarState({ navigationOpen: true }), readiness: "shell", apiResponses: [{ path: "/api/v1/state", status: 200, body: snapshotState({ paused: false }) }], steps: [{ kind: "wait", target: { selector: '.global-controls button[aria-label^="Pause"]' }, state: "visible" }], readyMarker: 'section[aria-label="General"]' },
  { id: "native-7f-settings-factories", route: "/settings", sidebar: sidebarState({ navigationOpen: true }), readiness: "shell", readyMarker: 'section[aria-label="General"]' },
  {
    id: "settings-models-expanded",
    route: "/settings",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "click", target: { role: "button", name: "Allowed models", exact: true } }],
    readyMarker: '[data-column="allowed-models"][data-ready="true"]'
  },
  {
    id: "settings-model-error",
    route: "/settings",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "click", target: { role: "button", name: "Allowed models", exact: true } }],
    apiResponses: [{ path: "/api/v1/models/pi", status: 503, body: '{"message":"Catalog unavailable","problems":[]}' }],
    expectedText: "Catalog unavailable",
    readyMarker: '[data-column="allowed-models"][data-ready="true"]'
  },
  {
    id: "settings-inline-422",
    route: "/settings",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    steps: [{ kind: "click", target: { role: "checkbox", name: "Notify me when a run needs me", exact: true } }],
    apiResponses: [{ path: "/api/v1/settings/patch", status: 422, body: '{"message":"Settings rejected","problems":["Choose an allowed model"]}' }],
    expectedText: "Settings rejected",
    readyMarker: 'section[aria-label="General"]'
  }
];
