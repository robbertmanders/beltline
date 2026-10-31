import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { answer, contextFor, fixture, mockDaemon, renderAt, type DaemonRequest } from "./beltsTestKit";
import type { Handler } from "../../test/daemon";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import type { BeltAction, BeltsResponse } from "../../api/wire.gen";
import { libraryChanged } from "../../test/daemon";

/** The belt editor's shadow tests: the switch between live and the shadow, saving a shadow, and ending the test. */

beforeEach(() => {
  localStorage.clear();
});

const issues = fixture.belts.find(record => record.shadow !== undefined)!;
const docs = fixture.belts.find(record => record.belt.id === "docs")!;
const where = () => screen.getByLabelText("Location").textContent ?? "";
const tag = () => document.querySelector(".belt-editor-title .paper-tag")?.textContent;

/** The context a shadow draft gets: the daemon's differences from live beside the usual menus. */
const shadowContext = (request: DaemonRequest) => json({
  ...contextFor(request.body.draft),
  ...(request.body.shadow ? { shadowDifferences: issues.shadow!.differences } : {})
});

describe("shadow tests in the belt editor", () => {
  it("shows the shadow with the daemon's stage tags, and keeps live's name and triggers out of reach", async () => {
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext });
    renderAt(`/belts?belt=${issues.belt.id}&shadow=1`);
    await waitFor(() => expect(tag()).toBe(`Belt · shadow v${issues.shadow!.version} · running`));
    const bar = screen.getByRole("group", { name: "Shadow test" });
    expect(within(bar).getByRole("button", { name: `Live · v${issues.belt.version}` }).getAttribute("aria-pressed")).toBe("false");
    expect(within(bar).getByText(issues.shadow!.caption)).toBeTruthy();
    await waitFor(() => expect(document.querySelector("[data-stage='build'] .shadow-diff__tag")?.textContent).toBe("Changed"));
    expect(screen.getByLabelText("Belt name").matches(":disabled")).toBe(true);
    expect(daemon.contextReads().at(-1)?.body.shadow).toBe(true);
    expect(screen.queryByRole("button", { name: "Copy belt" })).toBeNull();
    expect(screen.getByRole("button", { name: "Accept shadow…" })).toBeTruthy();
  });

  it("starts a shadow from the live version, and Start shadow test saves it as a shadow version", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext,
      [`/api/v1/belts/${docs.belt.id}/shadow/save`]: () => json(answer("belt-shadow-save.json"))
    });
    renderAt(`/belts?belt=${docs.belt.id}`);
    await waitFor(() => expect(tag()).toBe(`Belt · v${docs.belt.version} · running`));
    await user.click(await screen.findByRole("button", { name: "Create shadow…" }));
    await waitFor(() => expect(tag()).toBe("New shadow"));
    expect(where()).toContain("shadow=1");
    await user.click(screen.getByRole("button", { name: "Start shadow test" }));
    await waitFor(() => expect(daemon.posts("/shadow/save")).toHaveLength(1));
    expect(daemon.posts("/shadow/save")[0]!.body.belt.id).toBe(docs.belt.id);
    expect(daemon.posts("/api/v1/belts/save")).toHaveLength(0);
  });

  it("discarding a shadow not yet saved goes back to the live version", async () => {
    const user = userEvent.setup();
    mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext });
    renderAt(`/belts?belt=${docs.belt.id}&shadow=1`);
    await waitFor(() => expect(tag()).toBe("New shadow"));
    await user.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(tag()).toBe(`Belt · v${docs.belt.version} · running`));
    expect(where()).not.toContain("shadow=1");
  });

  it("accepts the shadow from its sheet, with the test's figures beside the button", async () => {
    const user = userEvent.setup();
    const figures = [{ id: "firstTry", label: "First try", live: { value: "71%", caption: "34 of 48 visits", bar: 0.71 },
      shadow: { value: "82%", caption: "37 of 45 visits", bar: 0.82 }, delta: { label: "+11 pts", tone: "better", caption: "Shadow against live" } }];
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext,
      "/api/v1/views/insights": () => json({ ...JSON.parse(goldenText("views-insights-belt.json")), shadow: { figures } }),
      [`/api/v1/belts/${issues.belt.id}/shadow/accept`]: () => json({ resource: { ...issues, shadow: undefined }, warnings: [], problems: [] })
    });
    renderAt(`/belts?belt=${issues.belt.id}&shadow=1`);
    await user.click(await screen.findByRole("button", { name: "Accept shadow…" }));
    const sheet = await screen.findByRole("dialog", { name: `Accept ${issues.shadow!.label.toLowerCase()}?` });
    await waitFor(() => expect(within(sheet).getByText("82%")).toBeTruthy());
    expect(within(sheet).getByText("+11 pts")).toBeTruthy();
    await user.click(within(sheet).getByRole("button", { name: "Accept shadow" }));
    await waitFor(() => expect(daemon.posts("/shadow/accept")).toHaveLength(1));
    await waitFor(() => expect(where()).not.toContain("shadow=1"));
  });

  it("disables Create shadow with the Settings explanation while shadow tests are off, and starts none", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextWithAction({ id: "shadow-create", enabled: false, reason: shadowTestsOff }) });
    renderAt(`/belts?belt=${docs.belt.id}`);
    await waitFor(() => expect(tag()).toBe(`Belt · v${docs.belt.version} · running`));
    const create = await screen.findByRole("button", { name: "Create shadow…" });
    expect((create as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(shadowTestsOff)).toBeTruthy();
    await user.click(create);
    expect(daemon.posts("/shadow/save")).toHaveLength(0);
    expect(tag()).toBe(`Belt · v${docs.belt.version} · running`);
  });

  it("refuses a direct shadow address with no test, keeping the draft and its Start disabled", async () => {
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextWithAction({ id: "shadow-create", enabled: false, reason: shadowTestsOff }) });
    renderAt(`/belts?belt=${docs.belt.id}&shadow=1`);
    await waitFor(() => expect(tag()).toBe("New shadow"));
    const start = await screen.findByRole("button", { name: "Start shadow test" });
    expect((start as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(shadowTestsOff)).toBeTruthy();
    expect(daemon.posts("/shadow/save")).toHaveLength(0);
  });

  it("keeps an open unsaved shadow draft when a refreshed answer forbids starting it, and starts it again once allowed", async () => {
    let refused = false;
    const list = JSON.parse(goldenText("belts.json")) as BeltsResponse;
    const daemon = mockDaemon({
      "/api/v1/belts": () => refused ? json({ ...list, belts: list.belts.map(record => (record.belt ?? record.draft).id === docs.belt.id ? { ...record, revision: "refused" } : record) }) : json(goldenText("belts.json")),
      "/api/v1/belts/context": request => json({ ...contextFor(request.body.draft), actions: [...contextFor(request.body.draft).actions, ...(refused ? [{ id: "shadow-create", enabled: false, reason: shadowTestsOff }] : [])] }),
      [`/api/v1/belts/${docs.belt.id}/shadow/save`]: () => json(answer("belt-shadow-save.json"))
    });
    renderAt(`/belts?belt=${docs.belt.id}&shadow=1`);
    await waitFor(() => expect(tag()).toBe("New shadow"));
    expect((await screen.findByRole("button", { name: "Start shadow test" }) as HTMLButtonElement).disabled).toBe(false);

    refused = true;
    libraryChanged(2);
    await new Promise(resolve => setTimeout(resolve, 800));

    await waitFor(() => expect((screen.getByRole("button", { name: "Start shadow test" }) as HTMLButtonElement).disabled).toBe(true));
    expect(screen.getByText(shadowTestsOff)).toBeTruthy();
    expect(tag()).toBe("New shadow");

    refused = false;
    libraryChanged(3);
    await waitFor(() => expect((screen.getByRole("button", { name: "Start shadow test" }) as HTMLButtonElement).disabled).toBe(false));
    expect(daemon.posts("/shadow/save")).toHaveLength(0);
  });

  it("drops the Compare link with Insights off, while the accept sheet keeps the test's figures", async () => {
    const user = userEvent.setup();
    const figures = [{ id: "firstTry", label: "First try", live: { value: "71%", caption: "34 of 48 visits", bar: 0.71 },
      shadow: { value: "82%", caption: "37 of 45 visits", bar: 0.82 }, delta: { label: "+11 pts", tone: "better", caption: "Shadow against live" } }];
    mockDaemon({
      "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext,
      "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights: false, enableShadowTests: true }),
      "/api/v1/views/insights": () => json({ ...JSON.parse(goldenText("views-insights-belt.json")), shadow: { figures } })
    });
    renderAt(`/belts?belt=${issues.belt.id}&shadow=1`);
    await user.click(await screen.findByRole("button", { name: "Accept shadow…" }));
    const sheet = await screen.findByRole("dialog", { name: `Accept ${issues.shadow!.label.toLowerCase()}?` });
    await waitFor(() => expect(within(sheet).getByText("82%")).toBeTruthy());
    expect(screen.queryByRole("link", { name: "Compare on Insights →" })).toBeNull();
  });

  it("keeps Create shadow, its edits and Accept working with Insights off", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": shadowContext,
      "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights: false, enableShadowTests: true }),
      [`/api/v1/belts/${docs.belt.id}/shadow/save`]: () => json(answer("belt-shadow-save.json")),
      [`/api/v1/belts/${issues.belt.id}/shadow/accept`]: () => json({ resource: { ...issues, shadow: undefined }, warnings: [], problems: [] })
    });
    renderAt(`/belts?belt=${docs.belt.id}`);
    await waitFor(() => expect(tag()).toBe(`Belt · v${docs.belt.version} · running`));
    await user.click(await screen.findByRole("button", { name: "Create shadow…" }));
    await waitFor(() => expect(tag()).toBe("New shadow"));
    await user.click(screen.getByRole("button", { name: "Start shadow test" }));
    await waitFor(() => expect(daemon.posts("/shadow/save")).toHaveLength(1));
  });
});

/** A shadow test's explanation for a command the daemon refuses while Settings keeps shadow tests off. */
const shadowTestsOff = "Enable shadow tests in Settings → Experimental to start a new test.";

/** The context a draft gets, with one more action from the daemon. */
function contextWithAction(action: BeltAction): Handler {
  return request => json({ ...contextFor(request.body.draft), actions: [...contextFor(request.body.draft).actions, action] });
}
