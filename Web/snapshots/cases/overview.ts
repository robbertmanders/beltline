import { shellCases, sidebarState, type PreCaptureStep, type SnapshotCase } from "../caseTypes";
import { snapshotState } from "../stateFixture";

/** The Overview once its floor is drawn under the summary, scrolled to it, after the given steps. */
const floorCase = (id: string, readyMarker: string, navigationOpen: boolean, extra: readonly PreCaptureStep[] = []): SnapshotCase => ({
  id,
  route: "/overview",
  sidebar: sidebarState({ navigationOpen }),
  readiness: "shell",
  steps: [{ kind: "wait", target: { selector: ".box-floor" }, state: "visible" }, ...extra],
  readyMarker
});

export const overviewCases: SnapshotCase[] = [
  ...shellCases("overview", "/overview"),
  floorCase("overview-floor-open", ".box-floor", true),
  floorCase("overview-floor-closed", ".box-floor", false),
  // Pinning the issues belt keeps its way through the factory drawn; a pinned belt lights its tracks and dims the rest.
  floorCase("overview-floor-pinned", ".box-belt.is-pinned[data-belt='issues']", true, [
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
