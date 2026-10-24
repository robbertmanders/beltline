import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { defaultPreferences, preferencesStorageKey, savePreferences, type WebPreferences } from "../../state/preferences";
import type { OverviewResponse } from "../../api/wire.gen";
import { goldenText } from "../../test/golden";
import { fixtureRoute, renderRoutes, stubDaemon } from "../../test/daemon";

const overview = (): OverviewResponse => JSON.parse(goldenText("views-overview.json")) as OverviewResponse;
const location = () => screen.getByLabelText("Location").textContent;

/** Opens Overview on the Floor, where the boxes, belts and tags it navigates from are drawn. */
async function openFloor() {
  const user = userEvent.setup();
  stubDaemon({ "/api/v1/views/overview": fixtureRoute("views-overview.json"), "/api/v1/views/work": fixtureRoute("views-work.json") });
  renderRoutes("/overview");
  await screen.findByRole("heading", { level: 1, name: "Overview" });
  await user.click(screen.getByRole("button", { name: "Floor" }));
  return user;
}

beforeEach(() => {
  localStorage.clear();
});

describe("Overview navigation", () => {
  it("opens active Work filtered to a merged stage, with the search cleared", async () => {
    const user = await openFloor();
    const box = overview().floor.boxes[0]!;
    await user.click(document.querySelector<HTMLButtonElement>(`[data-stage='${box.id}'] .stage-box`)!);
    await waitFor(() => expect(location()).toBe(`/work?stage=${encodeURIComponent(box.id)}`));
  });

  it("opens active Work for an entrance's belt with its stage and search cleared", async () => {
    const user = await openFloor();
    await user.click(document.querySelector<HTMLButtonElement>(".box-belt[data-belt='issues'] .box-belt-runs")!);
    await waitFor(() => expect(location()).toBe("/work?belt=issues"));
  });

  it("opens a Needs you row at its decision stage, with the run id encoded", async () => {
    stubDaemon({ "/api/v1/views/overview": fixtureRoute("views-overview.json"), "/api/v1/views/work": fixtureRoute("views-work.json") });
    const row = overview().today.needsYou.rows[0];
    renderRoutes("/overview");
    await screen.findByRole("heading", { level: 1, name: "Overview" });
    const needs = await screen.findByRole("region", { name: "Needs you" });
    await userEvent.setup().click(needs.querySelector<HTMLButtonElement>(".overview-row")!);
    const expected = `/runs/${encodeURIComponent(row!.runId)}${row!.openStage ? `?stage=${encodeURIComponent(row!.openStage)}` : ""}`;
    await waitFor(() => expect(location()).toBe(expected));
  });

  it("opens a Floor chip at its stage when a person is needed, and at the run otherwise", async () => {
    const user = await openFloor();
    const chip = overview().floor.boxes!.flatMap(box => box.chips)[0]!;
    await user.click(document.querySelector<HTMLButtonElement>(".floor-tag")!);
    const expected = `/runs/${encodeURIComponent(chip.runId)}${chip.openStage ? `?stage=${encodeURIComponent(chip.openStage)}` : ""}`;
    await waitFor(() => expect(location()).toBe(expected));
  });

  it("keeps stored Work and shell folds unchanged across Overview modes and a navigation into Work", async () => {
    const user = userEvent.setup();
    stubDaemon({ "/api/v1/views/overview": fixtureRoute("views-overview.json"), "/api/v1/views/work": fixtureRoute("views-work.json") });
    const stored: WebPreferences = {
      ...defaultPreferences,
      runsFolded: { repo: ["acme/web"] },
      watchingFolded: { repo: ["acme/billing"] },
      navigationFolded: { "workspace:agents": true },
      listFolded: { "belts:archived": true }
    };
    savePreferences(stored);
    renderRoutes("/overview");
    await screen.findByRole("heading", { level: 1, name: "Overview" });
    await user.click(screen.getByRole("button", { name: "Summary" }));
    await user.click(screen.getByRole("button", { name: "Floor" }));
    await user.click(screen.getByRole("link", { name: "Work" }));
    await screen.findByRole("heading", { level: 1, name: "Work" });
    const saved = JSON.parse(localStorage.getItem(preferencesStorageKey)!) as WebPreferences;
    expect(saved.runsFolded).toEqual(stored.runsFolded);
    expect(saved.watchingFolded).toEqual(stored.watchingFolded);
    expect(saved.navigationFolded).toEqual(stored.navigationFolded);
    expect(saved.listFolded).toEqual(stored.listFolded);
  });
});
