import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { InsightsEvolutionView, InsightsView } from "../../api/wire.gen";
import { goldenText } from "../../test/golden";
import { json } from "../../test/response";
import { renderPage, stubDaemon, type Handler } from "../../test/daemon";

const insightsPath = "/api/v1/views/insights";
const everyBelt = (): InsightsView => JSON.parse(goldenText("views-insights.json")) as InsightsView;
const oneBelt = (): InsightsView => JSON.parse(goldenText("views-insights-belt.json")) as InsightsView;
const evolutionPath = "/api/v1/views/insights/evolution";
const evolution = (): InsightsEvolutionView => JSON.parse(goldenText("views-insights-evolution.json")) as InsightsEvolutionView;

/** An archived belt the daemon offers once Show archived is on, with its own row and picker entry. */
const archivedBelt = "retired-belt";
const archivedLabel = "Retired belt";

/** The overview with the archived belt added, as the daemon answers with showArchived=1. */
const withArchived = (): InsightsView => {
  const base = everyBelt();
  const row = { ...base.beltRows[0]!, id: archivedBelt, name: archivedLabel, runs: 4 };
  return { ...base, belts: [...base.belts, { id: archivedBelt, label: archivedLabel }], beltRows: [...base.beltRows, row], caption: `${base.beltRows.length + 1} belts with runs` };
};

/** The detail of the archived belt: the picker keeps it chosen, and its figures are the belt golden's. */
const archivedDetail = (): InsightsView => ({
  ...oneBelt(), belt: archivedBelt, title: archivedLabel, belts: [...everyBelt().belts, { id: archivedBelt, label: archivedLabel }]
});

/**
 * The Insights daemon: a belt query gets the belt golden (the archived belt's detail for its own id), and any other
 * the overview of every belt, with the archived belt once showArchived=1 asks for it.
 */
function insightsDaemon() {
  const route: Handler = request => {
    const params = new URL(request.path, "http://127.0.0.1").searchParams;
    const belt = params.get("belt");
    if (belt === archivedBelt) return json(archivedDetail());
    if (belt) return json(oneBelt());
    return json(params.get("showArchived") === "1" ? withArchived() : everyBelt());
  };
  const daemon = stubDaemon({ [insightsPath]: route, [evolutionPath]: () => json(evolution()) });
  return { ...daemon, queries: () => daemon.requests.filter(request => request.path.startsWith(insightsPath)).map(request => request.path) };
}

afterEach(() => {
  cleanup();
});

describe("Insights page", () => {
  it("lists every belt the daemon sends, and opens a belt from its row", async () => {
    const daemon = insightsDaemon();
    renderPage("/insights");
    const table = await screen.findByRole("table", { name: "Belts" });
    const names = within(table).getAllByRole("row").slice(1).map(row => within(row).getAllByRole("cell")[0]?.textContent);
    expect(names).toEqual(everyBelt().beltRows.map(row => row.name));
    const first = everyBelt().beltRows[0]!;
    await userEvent.setup().click(within(table).getByText(first.name));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${evolutionPath}?belt=${first.id}`));
    expect(await screen.findByRole("button", { name: "Lead time" })).toBeTruthy();
  });

  it("marks the page as experimental, on every belt and on one", async () => {
    insightsDaemon();
    renderPage("/insights");
    await screen.findByRole("table", { name: "Belts" });
    expect(document.querySelector(".page-heading")?.textContent).toContain("EXPERIMENTAL");
    cleanup();
    renderPage(`/insights?belt=${everyBelt().beltRows[0]!.id}`);
    await screen.findByRole("button", { name: "Lead time" });
    expect(document.querySelector(".page-heading")?.textContent).toContain("EXPERIMENTAL");
  });

  it("shows the belt's stages in the daemon's order", async () => {
    insightsDaemon();
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages`);
    const table = await screen.findByRole("table", { name: "Stages" });
    const rows = table.querySelectorAll("tbody tr[data-stage]");
    expect([...rows].map(row => row.getAttribute("data-stage"))).toEqual(oneBelt().stages.map(stage => stage.key));
    const approval = oneBelt().stages.find(stage => stage.kindLabel !== "")!;
    const approvalRow = table.querySelector(`tr[data-stage="${approval.key}"]`)!;
    expect(within(approvalRow as HTMLElement).getByText(approval.kindLabel)).toBeTruthy();
    expect(approvalRow.getAttribute("aria-expanded")).toBeNull();
    expect(within(table).getAllByRole("columnheader").map(cell => cell.textContent))
      .toEqual(["Stage", "Visits", "Visits / run", "Tries / visit", "First try", "Median time", "Cost"]);
    const first = oneBelt().stages[0]!;
    const cells = table.querySelector(`tr[data-stage="${first.key}"]`)!.querySelectorAll("td");
    expect([...cells].slice(1, 4).map(cell => cell.textContent)).toEqual([String(first.figures.visits), first.figures.visitsPerRun, first.figures.tries]);
  });

  it("opens a stage to show each of its models, and closes it again", async () => {
    insightsDaemon();
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages`);
    const table = await screen.findByRole("table", { name: "Stages" });
    const stage = oneBelt().stages.find(item => item.models.length > 1)!;
    const row = table.querySelector(`tr[data-stage="${stage.key}"]`) as HTMLElement;
    const user = userEvent.setup();
    await user.click(row);
    expect(row.getAttribute("aria-expanded")).toBe("true");
    const shown = () => [...table.querySelectorAll(".insights-model-name")].map(name => name.textContent);
    expect(shown()).toEqual(stage.models.map(model => model.model));
    expect(table.querySelector(".insights-model-note")).toBeNull();
    expect(screen.queryByText(/Best is the highest first try/)).toBeNull();
    await user.click(row);
    expect(row.getAttribute("aria-expanded")).toBe("false");
    expect(shown()).toEqual([]);
  });

  it("asks the daemon for the period the control picks, and leaves all time out of the request", async () => {
    const daemon = insightsDaemon();
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages`);
    await screen.findByRole("table", { name: "Stages" });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "7 days" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${oneBelt().belt}&period=7d`));
    await user.click(screen.getByRole("button", { name: "All time" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${oneBelt().belt}`));
  });

  it("asks for the version the control picks, and starts at every version when the belt changes", async () => {
    const daemon = insightsDaemon();
    const belt = oneBelt().belt;
    renderPage(`/insights?belt=${belt}&view=stages`);
    await screen.findByRole("table", { name: "Stages" });
    const user = userEvent.setup();
    const choice = oneBelt().versions.find(item => item.value !== 0)!;
    await user.selectOptions(screen.getByRole("combobox", { name: "Version" }), choice.label);
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${belt}&version=${choice.value}`));
    await user.selectOptions(screen.getByRole("combobox", { name: "Belt" }), "All belts");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(insightsPath));
  });

  it("shows the daemon's refusal when the belt is unknown", async () => {
    stubDaemon({ [insightsPath]: () => json(goldenText("error-not-found.json"), 404), [evolutionPath]: () => json(goldenText("error-not-found.json"), 404) });
    renderPage("/insights?belt=gone");
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText("Unable to load Insights")).toBeTruthy();
  });
});

describe("Insights page, archived belts", () => {
  const archiveCheckbox = () => screen.getByRole("checkbox", { name: "Show archived" }) as HTMLInputElement;
  const pickerOptions = () => within(screen.getByRole("combobox", { name: "Belt" })).getAllByRole("option").map(option => option.textContent);
  const rowNames = (table: HTMLElement) => within(table).queryAllByRole("row").slice(1).map(row => within(row).getAllByRole("cell")[0]?.textContent);

  it("lists exactly the belts and rows the daemon sends, and starts with Show archived off", async () => {
    const daemon = insightsDaemon();
    renderPage("/insights");
    const table = await screen.findByRole("table", { name: "Belts" });
    expect(rowNames(table)).toEqual(everyBelt().beltRows.map(row => row.name));
    expect(pickerOptions()).toEqual(["All belts", ...everyBelt().belts.map(belt => belt.label)]);
    expect(archiveCheckbox().checked).toBe(false);
    expect(daemon.queries().at(-1)).toBe(insightsPath);
  });

  it("asks with showArchived=1 when checked, shows the archived belt, and drops it again when unchecked", async () => {
    const daemon = insightsDaemon();
    renderPage("/insights");
    await screen.findByRole("table", { name: "Belts" });
    const user = userEvent.setup();
    await user.click(archiveCheckbox());
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?showArchived=1`));
    expect(archiveCheckbox().checked).toBe(true);
    await waitFor(() => expect(rowNames(screen.getByRole("table", { name: "Belts" }))).toContain(archivedLabel));
    expect(pickerOptions()).toContain(archivedLabel);
    await user.click(archiveCheckbox());
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(insightsPath));
    await waitFor(() => expect(rowNames(screen.getByRole("table", { name: "Belts" }))).not.toContain(archivedLabel));
    expect(pickerOptions()).not.toContain(archivedLabel);
  });

  it("restores a checked Show archived from a fresh URL", async () => {
    const daemon = insightsDaemon();
    renderPage("/insights?showArchived=1");
    expect(rowNames(await screen.findByRole("table", { name: "Belts" }))).toContain(archivedLabel);
    expect(archiveCheckbox().checked).toBe(true);
    expect(daemon.queries().at(-1)).toBe(`${insightsPath}?showArchived=1`);
  });

  it("keeps Show archived through the belt, version, shadow and period controls", async () => {
    const daemon = insightsDaemon();
    const belt = oneBelt().belt;
    renderPage(`/insights?belt=${belt}&view=stages&showArchived=1`);
    await screen.findByRole("table", { name: "Stages" });
    expect(archiveCheckbox().checked).toBe(true);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "7 days" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${belt}&period=7d&showArchived=1`));
    const version = oneBelt().versions.find(item => item.value !== 0)!;
    await user.selectOptions(screen.getByRole("combobox", { name: "Version" }), version.label);
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${belt}&version=${version.value}&period=7d&showArchived=1`));
    const shadow = oneBelt().versions.find(item => item.shadow)!;
    await user.selectOptions(screen.getByRole("combobox", { name: "Version" }), `shadow:${shadow.shadow}`);
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${belt}&shadow=${shadow.shadow}&period=7d&showArchived=1`));
    await user.selectOptions(screen.getByRole("combobox", { name: "Belt" }), "All belts");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?period=7d&showArchived=1`));
    expect(archiveCheckbox().checked).toBe(true);
  });

  it("keeps an archived belt's detail when Show archived is switched off, and filters the list on All belts", async () => {
    const daemon = insightsDaemon();
    renderPage(`/insights?belt=${archivedBelt}&view=stages&showArchived=1`);
    await screen.findByRole("table", { name: "Stages" });
    const select = screen.getByRole("combobox", { name: "Belt" }) as HTMLSelectElement;
    expect(select.value).toBe(archivedBelt);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(archivedLabel);
    const user = userEvent.setup();
    await user.click(archiveCheckbox());
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${archivedBelt}`));
    expect(select.value).toBe(archivedBelt);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(archivedLabel);
    expect(screen.getByRole("table", { name: "Stages" })).toBeTruthy();
    await user.selectOptions(select, "All belts");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(insightsPath));
    await waitFor(() => expect(pickerOptions()).not.toContain(archivedLabel));
    expect(rowNames(await screen.findByRole("table", { name: "Belts" }))).not.toContain(archivedLabel);
  });

  it("renders an empty overview as a table with no rows and the daemon's caption", async () => {
    stubDaemon({ [insightsPath]: () => json({ ...everyBelt(), belts: [], beltRows: [], caption: "0 belts with runs" }) });
    renderPage("/insights");
    const table = await screen.findByRole("table", { name: "Belts" });
    expect(table.querySelectorAll("tbody tr")).toHaveLength(0);
    expect(screen.getByText("0 belts with runs")).toBeTruthy();
    expect(pickerOptions()).toEqual(["All belts"]);
  });
});

describe("Insights page, shadow tests", () => {
  const figures = (["firstTry", "shipRate", "leadTime", "cost"] as const).map((id, index) => ({
    id, label: ["First try", "Ship rate", "Lead time", "Cost per run"][index]!,
    live: { value: "71%", caption: "34 of 48 visits", bar: 0.71 }, shadow: { value: "82%", caption: "37 of 45 visits", bar: 0.82 },
    delta: { label: "+11 pts", tone: "better" as const, caption: "Shadow against live" }
  }));
  const figuresOf = { visits: 4, visitsPerRun: "1.50", lowSample: true, medianTime: "2m 0s", firstTry: { count: 3, of: 4, percent: "75%" }, tries: "1.25", cost: "–", costCaption: "" };
  const shadowView = (): InsightsView => ({
    ...oneBelt(), stages: [],
    shadow: {
      test: "t1", running: true, dates: "Runs started since Oct 2", caption: "Only runs that started during the test count.",
      live: { label: "Live v1", finished: 9, going: 2, caption: "9 finished · 2 going" },
      shadow: { label: "Shadow v2", finished: 8, going: 3, caption: "8 finished · 3 going" },
      figures,
      stages: [{ key: "build", name: "Build", kindLabel: "", changed: true, live: figuresOf, shadow: { ...figuresOf, tries: "1.00" },
        tones: { visitsPerRun: "flat", medianTime: "flat", firstTry: "flat", tries: "better", cost: "none" } }],
      onlyShadow: [{ key: "plan", name: "Plan check", caption: "", kindLabel: "", figures: figuresOf, models: [] }], onlyLive: []
    }
  });

  it("offers the belt's shadow tests as versions and asks for the chosen one", async () => {
    const daemon = insightsDaemon();
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages`);
    await screen.findByRole("table", { name: "Stages" });
    const choice = oneBelt().versions.find(item => item.shadow)!;
    await userEvent.setup().selectOptions(screen.getByLabelText("Version"), `shadow:${choice.shadow}`);
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${oneBelt().belt}&shadow=${choice.shadow}`));
  });

  it("compares the two versions figure by figure and stage by stage, without the period buttons", async () => {
    stubDaemon({ [insightsPath]: () => json(shadowView()) });
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages&version=shadow%3At1`);
    const shared = await screen.findByRole("table", { name: "Shared stages" });
    expect(screen.getByText("Shadow v2")).toBeTruthy();
    for (const figure of figures) expect(screen.getByRole("region", { name: figure.label })).toBeTruthy();
    expect(within(shared).getByText("Changed")).toBeTruthy();
    expect(within(shared).getByText("1.00").className).toContain("is-better");
    expect(screen.getByRole("table", { name: "Only in shadow v2" })).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Period" })).toBeNull();
    expect(screen.getByText("Runs started since Oct 2")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Accept shadow…" })).toBeTruthy();
  });

  it("shows an ended shadow comparison with its result while shadow tests are off, and no decision", async () => {
    const ended = shadowView();
    ended.shadow = { ...ended.shadow!, running: false, result: "accepted" };
    stubDaemon({
      [insightsPath]: () => json(ended),
      "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights: true, enableShadowTests: false })
    });
    renderPage(`/insights?belt=${oneBelt().belt}&view=stages&version=shadow%3At1`);
    const shared = await screen.findByRole("table", { name: "Shared stages" });
    expect(screen.getByText("Shadow v2")).toBeTruthy();
    expect(within(shared).getByText("Changed")).toBeTruthy();
    expect(screen.getByText("accepted")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Accept shadow…" })).toBeNull();
  });
});

describe("Insights page, evolution", () => {
  it("opens a belt on Evolution with the daemon's figures, and switches to its stages", async () => {
    const daemon = insightsDaemon();
    const belt = oneBelt().belt;
    renderPage(`/insights?belt=${belt}`);
    for (const figure of evolution().figures) expect(await screen.findByRole("button", { name: figure.label })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Lead time" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText(evolution().split)).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(evolution().title);
    expect(screen.queryByRole("group", { name: "Period" })).toBeNull();
    expect(screen.queryByRole("combobox", { name: "Version" })).toBeNull();
    expect(daemon.queries()).toEqual([`${evolutionPath}?belt=${belt}`]);
    await userEvent.setup().click(screen.getByRole("button", { name: "Stages" }));
    expect(await screen.findByRole("table", { name: "Stages" })).toBeTruthy();
    expect(daemon.queries().at(-1)).toBe(`${insightsPath}?belt=${belt}`);
  });

  it("asks for weeks, separate versions and where recent starts, and keeps the scale and figure to the page", async () => {
    const daemon = insightsDaemon();
    const belt = oneBelt().belt;
    renderPage(`/insights?belt=${belt}&from=2`);
    await screen.findByRole("button", { name: "Lead time" });
    expect(daemon.queries().at(-1)).toBe(`${evolutionPath}?belt=${belt}&from=2`);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Keep apart" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${evolutionPath}?belt=${belt}&separate=1`));
    await user.click(screen.getByRole("button", { name: "Week" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${evolutionPath}?belt=${belt}&by=week&separate=1`));
    expect(screen.queryByRole("group", { name: "Small versions" })).toBeNull();
    const asked = daemon.queries().length;
    await user.click(screen.getByRole("button", { name: "Linear" }));
    await user.click(screen.getByRole("button", { name: "Ship rate" }));
    expect(screen.getByRole("button", { name: "Ship rate" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("region", { name: "Ship rate by belt version" })).toBeTruthy();
    expect(daemon.queries()).toHaveLength(asked);
  });
});
