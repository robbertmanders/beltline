import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TestApp } from "../app/App";
import { apiQueryKey } from "../api/resources";
import { goldenText } from "../test/golden";
import { json } from "../test/response";
import { fixtureRoute, refusal, renderPage, stubDaemon, type Handler } from "../test/daemon";
import type { SettingsResponse } from "../api/wire.gen";

/** The golden holds every decision; `?filter=review` keeps only those that need a check, as the daemon does. */
const decisionsRoute: Handler = request => {
  const response = JSON.parse(goldenText("afk-decisions.json")) as { decisions: Array<{ needsReview: boolean; status: string }>; filter: string };
  const filter = new URL(request.path, "http://127.0.0.1").searchParams.get("filter") ?? "review";
  if (filter !== "review") return json({ ...response, filter: "all" });
  return json({ ...response, filter: "review", decisions: response.decisions.filter(decision => decision.needsReview || decision.status === "deciding") });
};

/** The AFK page's daemon: decisions filtered as the daemon filters them, and the settings and models it reads. */
function daemonFor(routes: Record<string, Handler> = {}) {
  return stubDaemon({
    "/api/v1/afk/decisions": decisionsRoute,
    "/api/v1/settings": fixtureRoute("settings.json"),
    "/api/v1/models/pi": fixtureRoute("models.json"),
    ...routes
  });
}

async function renderAfk(routes: Record<string, Handler> = {}) {
  const daemon = daemonFor(routes);
  renderPage("/afk");
  await screen.findByRole("heading", { level: 1, name: "AFK" });
  await waitFor(() => expect(screen.queryByText("AFK decisions loaded")).toBeTruthy());
  return daemon;
}

/** Changes the settings the page shows from outside, as another page's save does. */
function RemoteSettingsProbe() {
  const queryClient = useQueryClient();
  useEffect(() => {
    const update = () => {
      const settings = JSON.parse(goldenText("settings.json")) as SettingsResponse;
      queryClient.setQueryData(apiQueryKey("/api/v1/settings"), { ...settings, afk: { ...settings.afk, guidance: "changed by another page", limit: 8 } });
    };
    window.addEventListener("test:settings-change", update);
    return () => window.removeEventListener("test:settings-change", update);
  }, [queryClient]);
  return null;
}

describe("AFK decisions", () => {
  it("renders the daemon's decisions, filtered by the daemon, with their labels and review controls", async () => {
    await renderAfk();
    expect(await screen.findByText("Could not decide at Wrap up: The session wrote no decision.")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Looks right" }).length).toBeGreaterThan(0);
    expect(screen.queryByText("Overruled, restarting Wrap up · Build")).toBeNull();
    await userEvent.setup().selectOptions(screen.getByRole("combobox", { name: "Decision filter" }), "all");
    expect(await screen.findByText("Overruled, restarting Wrap up · Build")).toBeTruthy();
    expect(screen.getByText("It was waiting because: held: needs a decision")).toBeTruthy();
    expect(screen.getByText("Checked")).toBeTruthy();
  });

  it("links only decisions whose run still exists", async () => {
    await renderAfk();
    const deciding = (await screen.findByText("Deciding at Wrap up…")).closest("li")!;
    expect(within(deciding).queryByRole("link", { name: "Open run" })).toBeNull();
    const failed = screen.getByText("Could not decide at Wrap up: The session wrote no decision.").closest("li")!;
    expect(within(failed).getByRole("link", { name: "Open run" }).getAttribute("href")).toBe("/runs/issues%3Aacme%2Fweb%2361?stage=wrap-up");
  });

  it("overrules an applied decision with the offered stage, the comments and the accepted warning", async () => {
    const decisions = JSON.parse(goldenText("afk-decisions.json")).decisions as Array<{ id: string; runId: string; overruleOffer?: { available: boolean; previous?: { stageId: string } } }>;
    const decision = decisions.find(item => item.overruleOffer?.available && item.overruleOffer.previous)!;
    const path = `/api/v1/runs/${encodeURIComponent(decision.runId)}/overrule-afk`;
    const daemon = await renderAfk({ [path]: fixtureRoute("run-overrule-afk.json") });
    const user = userEvent.setup();
    await user.selectOptions(screen.getByRole("combobox", { name: "Decision filter" }), "all");
    const overrule = (await screen.findAllByRole("button", { name: "Overrule" })).find(button => !(button as HTMLButtonElement).disabled)!;
    await user.click(overrule);
    const sheet = await screen.findByRole("dialog");
    const submit = within(sheet).getByRole("button", { name: "Overrule and restart" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    await user.type(within(sheet).getByLabelText("Comments"), "Use the smaller fix.");
    await user.click(within(sheet).getByLabelText("I understand, restart anyway"));
    await user.click(submit);
    await waitFor(() => expect(daemon.posts(path)).toHaveLength(1));
    expect(daemon.posts(path)[0]!.body).toEqual({ decision: decision.id, stage: decision.overruleOffer!.previous!.stageId, comments: "Use the smaller fix.", acceptedWarning: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("submits an allowance patch with the new value, and sends no review commands", async () => {
    const daemon = await renderAfk({ "/api/v1/settings/patch": fixtureRoute("settings.json") });
    const toggle = screen.getByRole("checkbox", { name: "Run failed stages again" }) as HTMLInputElement;
    expect(toggle.checked).toBe(true);
    await userEvent.setup().click(toggle);
    await waitFor(() => expect(daemon.posts("/api/v1/settings/patch")).toHaveLength(1));
    expect(daemon.posts("/api/v1/settings/patch")[0]!.body).toEqual({ afk: { rerunFailures: false } });
    expect(daemon.requests.some(request => /\/decide|overrule|\/api\/v1\/afk\/review/.test(request.path))).toBe(false);
  });

  it("shows a refused settings save inline and keeps the unsent guidance text", async () => {
    await renderAfk({ "/api/v1/settings/patch": refusal("Rejected settings", ["limit out of range"]) });
    const guidance = screen.getByLabelText("Keep in mind") as HTMLTextAreaElement;
    await userEvent.setup().type(guidance, "never approve billing");
    guidance.blur();
    expect(await screen.findByText("Rejected settings")).toBeTruthy();
    expect(screen.getByText("limit out of range")).toBeTruthy();
    expect((screen.getByLabelText("Keep in mind") as HTMLTextAreaElement).value).toBe("never approve billing");
  });

  it("preserves dirty guidance when a live settings update arrives", async () => {
    daemonFor();
    render(<TestApp initialEntries={["/afk"]}><RemoteSettingsProbe /></TestApp>);
    const guidance = await screen.findByLabelText("Keep in mind") as HTMLTextAreaElement;
    guidance.focus();
    fireEvent.change(guidance, { target: { value: "unsent guidance" } });
    await act(async () => { window.dispatchEvent(new Event("test:settings-change")); });
    expect(guidance.value).toBe("unsent guidance");
  });

  it("steps the per-stage limit, sending the patch", async () => {
    const daemon = await renderAfk({ "/api/v1/settings/patch": fixtureRoute("settings.json") });
    const group = await screen.findByRole("group", { name: "Decisions per stage" });
    expect(group.textContent).toMatch(/At most \d+ decisions? per stage of a run/);
    await userEvent.setup().click(screen.getByRole("button", { name: "More decisions per stage" }));
    await waitFor(() => expect(daemon.posts("/api/v1/settings/patch")).toHaveLength(1));
  });
});

describe("AFK review", () => {
  it("marks one decision, then all, as checked through the daemon", async () => {
    const user = userEvent.setup();
    const daemon = await renderAfk({ "/api/v1/afk/review": fixtureRoute("afk-review.json") });
    const bodies = () => daemon.posts("/api/v1/afk/review").map(request => (request.body ?? {}) as { ids?: string[] });
    await user.click((await screen.findAllByRole("button", { name: "Looks right" }))[0]!);
    await waitFor(() => expect(bodies().some(body => Array.isArray(body.ids) && body.ids.length === 1)).toBe(true));
    await user.click(await screen.findByRole("button", { name: "Mark all as checked" }));
    await waitFor(() => expect(bodies().some(body => body.ids === undefined)).toBe(true));
  });
});
