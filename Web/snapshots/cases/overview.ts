import { shellCases, sidebarState, type PreCaptureStep, type SnapshotCase } from "../caseTypes";
import { snapshotState } from "../stateFixture";

const floorMode: PreCaptureStep = { kind: "click", target: { role: "button", name: "Floor", exact: true } };

/** A view reached from the Overview's own mode control, once its drawing is on screen. */
const modeCase = (id: string, step: PreCaptureStep, readyMarker: string, navigationOpen: boolean, extra: readonly PreCaptureStep[] = []): SnapshotCase => ({
  id,
  route: "/overview",
  sidebar: sidebarState({ navigationOpen }),
  readiness: "shell",
  steps: [step, ...extra],
  readyMarker
});

export const overviewCases: SnapshotCase[] = [
  ...shellCases("overview", "/overview"),
  modeCase("overview-floor-open", floorMode, ".box-floor", true),
  modeCase("overview-floor-closed", floorMode, ".box-floor", false),
  // Pinning the issues belt keeps its way through the factory drawn; a pinned belt lights its tracks and dims the rest.
  modeCase("overview-floor-pinned", floorMode, ".box-belt.is-pinned[data-belt='issues']", true, [
    { kind: "click", target: { selector: ".box-belt[data-belt='issues'] .box-belt-pin" } }
  ]),
  {
    // 7h-overview-paused: the paused state is a fixture, so the one demo daemon is never paused for the other cases.
    id: "overview-paused",
    route: "/overview",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [{ path: "/api/v1/state", status: 200, body: snapshotState({ paused: true }) }]
  }
];
