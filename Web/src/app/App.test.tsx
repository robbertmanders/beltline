import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AppProviders, AppRoutes, TestApp } from "./App";
import { AppSocket } from "../test/setup";
import { defaultPreferences, preferencesStorageKey } from "../state/preferences";
import { goldenText } from "../test/golden";
import { json } from "../test/response";
import { fixtureRoute, history, refusal, renderRoutes, stubDaemon, type Handler } from "../test/daemon";

/** The golden state with its navigation's library fields set, as the daemon reports them. */
function stateWithLibrary(library: { offered: boolean; changedSinceBackup: boolean | null }): string {
  const state = JSON.parse(goldenText("state.json")) as { navigation: { library: unknown } };
  state.navigation.library = library;
  return JSON.stringify(state);
}

/** The app's daemon: goldens for every page's resource, with the routes a test changes. */
function daemonFor(routes: Record<string, Handler> = {}) {
  return stubDaemon({
    "/api/v1/views/overview": fixtureRoute("views-overview.json"),
    "/api/v1/views/work": fixtureRoute("views-work.json"),
    "/api/v1/views/run/issues%3Aacme%2Fweb%2361": fixtureRoute("views-run.json"),
    "/api/v1/belts": fixtureRoute("belts.json"),
    "/api/v1/agents": fixtureRoute("agents.json"),
    "/api/v1/afk/decisions": fixtureRoute("afk-decisions.json"),
    "/api/v1/settings": fixtureRoute("settings.json"),
    "/api/v1/library/repos": fixtureRoute("views-overview.json"),
    "/api/v1/pause": fixtureRoute("command-pause.json"),
    "/api/v1/afk": fixtureRoute("command-afk.json"),
    "/api/v1/check-now": fixtureRoute("command-check-now.json"),
    ...routes
  });
}

describe("route shell and placeholders", () => {
  it("opens the daemon-independent gallery in development, and treats it as not found in production", async () => {
    vi.stubEnv("DEV", true);
    try {
      // The gallery is a lazy import; under a parallel full run its chunk can take longer than findBy's default second.
      render(<TestApp initialEntries={["/_gallery"]} />);
      expect(await screen.findByRole("heading", { name: "Theme gallery" }, { timeout: 10000 })).toBeTruthy();
      expect(screen.queryByRole("navigation", { name: "Primary navigation" })).toBeNull();
    } finally {
      vi.unstubAllEnvs();
    }
    vi.stubEnv("DEV", false);
    try {
      daemonFor({ "/api/v1/version": refusal("Sign in", [], 401) });
      render(<AppProviders><TestApp initialEntries={["/_gallery"]} /></AppProviders>);
      expect(await screen.findByRole("heading", { level: 1, name: "Not found" })).toBeTruthy();
      expect(screen.queryByRole("link", { name: /gallery/i })).toBeNull();
      expect(screen.queryByRole("heading", { level: 1, name: "Sign in required" })).toBeNull();
    } finally {
      vi.unstubAllEnvs();
    }
  });

  it.each([
    ["/overview", "/api/v1/views/overview", "Overview"],
    ["/runs/issues%3Aacme%2Fweb%2361", "/api/v1/views/run/issues%3Aacme%2Fweb%2361", /Split invoicing into its own service$/],
    ["/belts", "/api/v1/belts", "Belts"],
    ["/agents", "/api/v1/agents", "Agents"],
    ["/afk", "/api/v1/afk/decisions", "AFK"],
    ["/settings", "/api/v1/settings", "Settings"]
  ])("loads the real %s page with its daemon resource, and the state and version", async (path, expectedRequest, title) => {
    const daemon = daemonFor();
    renderPage(path);
    expect(await screen.findByRole("heading", { level: 1, name: title })).toBeTruthy();
    await waitFor(() => expect(daemon.requests.some(request => request.path.split("?")[0]!.startsWith(expectedRequest))).toBe(true));
    expect(daemon.requests.some(request => request.path === "/api/v1/state")).toBe(true);
    expect(daemon.requests.some(request => request.path === "/api/v1/version")).toBe(true);
  });

  it("redirects / to Overview, and selects Work in navigation while a Run is open", async () => {
    daemonFor({ "/api/v1/views/run/id": fixtureRoute("views-run.json") });
    renderPage("/");
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeTruthy();
    cleanup();
    renderPage("/runs/id");
    await screen.findByRole("heading", { name: /Split invoicing into its own service$/ });
    expect(screen.getByRole("link", { name: "Work" }).getAttribute("aria-current")).toBe("page");
  });

  it("uses browser history through route navigation", async () => {
    daemonFor();
    const user = userEvent.setup();
    renderRoutes("/overview");
    await screen.findByRole("heading", { name: "Overview" });
    await user.click(screen.getByRole("link", { name: "Work" }));
    await screen.findByRole("heading", { name: "Work" });
    await user.click(screen.getByRole("link", { name: "Belts" }));
    await screen.findByRole("heading", { level: 1, name: "Belts" });
    await history(user, "Back");
    await screen.findByRole("heading", { name: "Work" });
    await history(user, "Back");
    await screen.findByRole("heading", { name: "Overview" });
    await history(user, "Forward");
    await screen.findByRole("heading", { name: "Work" });
  });

  it.each([true, false])("offers the Library destination only when the daemon's navigation does (%s)", async offered => {
    daemonFor({ "/api/v1/state": () => json(stateWithLibrary({ offered, changedSinceBackup: null })) });
    renderPage("/overview");
    const nav = await screen.findByRole("navigation", { name: "Primary navigation" });
    await waitFor(() => expect(within(nav).queryByRole("link", { name: /^Library/ }) !== null).toBe(offered));
  });

  it.each([true, false])("lists Insights only while the daemon's settings show it (%s)", async showInsights => {
    daemonFor({ "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights }) });
    renderPage("/overview");
    const nav = await screen.findByRole("navigation", { name: "Primary navigation" });
    await waitFor(() => expect(within(nav).queryByRole("link", { name: /^Insights/ }) !== null).toBe(showInsights));
  });

  it("says Insights is turned off on its route while it is hidden, and points to Settings", async () => {
    daemonFor({ "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights: false }) });
    renderPage("/insights");
    expect(await screen.findByRole("heading", { name: "Insights is turned off" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Settings → Experimental" }).getAttribute("href")).toBe("/settings");
  });

  it.each([[true, false], [false, true]])("gates Insights on its own switch, not shadow tests (Insights %s, shadow tests %s)", async (showInsights, enableShadowTests) => {
    daemonFor({ "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights, enableShadowTests }) });
    renderPage("/overview");
    const nav = await screen.findByRole("navigation", { name: "Primary navigation" });
    await waitFor(() => expect(within(nav).queryByRole("link", { name: /^Insights/ }) !== null).toBe(showInsights));
  });

  it("opens Insights by its direct route when its own switch is on, whatever shadow tests are set to", async () => {
    daemonFor({ "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), showInsights: true, enableShadowTests: false }) });
    renderPage("/insights");
    expect(await screen.findByRole("heading", { level: 1, name: "Insights" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Insights is turned off" })).toBeNull();
  });

  it.each([true, false])("shows the changed-since-backup mark only when the daemon says the library changed (%s)", async changedSinceBackup => {
    daemonFor({ "/api/v1/state": () => json(stateWithLibrary({ offered: true, changedSinceBackup })) });
    renderPage("/overview");
    await screen.findByRole("navigation", { name: "Primary navigation" });
    await waitFor(() => expect(screen.queryByText("Changed since backup") !== null).toBe(changedSinceBackup));
  });

  it("turns any API 401 into sign-in while keeping the shell navigation", async () => {
    daemonFor({ "/api/v1/views/work": refusal("Sign in", [], 401) });
    renderPage("/work");
    expect(await screen.findByRole("heading", { name: "Sign in required" })).toBeTruthy();
    expect(screen.getByText(/\/auth\?token=…/)).toBeTruthy();
    expect(screen.getByRole("navigation", { name: "Primary navigation" })).toBeTruthy();
  });
});

describe("global controls and visible state", () => {
  it("uses the daemon's action order and posts the exact toggle bodies", async () => {
    const daemon = daemonFor();
    const user = userEvent.setup();
    renderPage("/overview");
    const controls = await screen.findByLabelText("Global controls");
    await within(controls).findByRole("button", { name: "Pause everything" });
    const buttons = within(controls).getAllByRole("button").filter(button => button.textContent !== "Dismiss");
    expect(buttons.map(button => button.textContent)).toEqual(["Pause everything", "Fetch"]);
    await waitFor(() => expect(buttons[0]?.hasAttribute("disabled")).toBe(false));
    await user.click(buttons[0]!);
    await waitFor(() => expect(daemon.posts("/api/v1/pause")).toHaveLength(1));
    expect(daemon.posts("/api/v1/pause")[0]!.body).toEqual({ paused: true });
    await user.click(await screen.findByRole("switch", { name: "Global AFK" }));
    await waitFor(() => expect(daemon.posts("/api/v1/afk")).toHaveLength(1));
    expect(daemon.posts("/api/v1/afk")[0]!.body).toEqual({ enabled: true });
    await user.click(within(controls).getByRole("button", { name: "Fetch" }));
    await waitFor(() => expect(daemon.posts("/api/v1/check-now")).toHaveLength(1));
    expect(daemon.posts("/api/v1/check-now")[0]!.body).toEqual({});
  });

  it("shows AFK as a switch at the foot of the sidebar, with the daemon's setting and reason", async () => {
    const state = JSON.parse(stateWithLibrary({ offered: true, changedSinceBackup: true })) as { actions: Array<{ id: string; enabled: boolean; reason?: string | null }>; settings: { afkEnabled: boolean } };
    daemonFor({ "/api/v1/state": () => json({ ...state, settings: { ...state.settings, afkEnabled: true }, actions: state.actions.map(action => action.id === "afk" ? { ...action, enabled: false, reason: "AFK is locked by policy." } : action) }) });
    renderPage("/overview");
    const afk = await screen.findByRole("switch", { name: "Global AFK" });
    expect(afk.getAttribute("aria-checked")).toBe("true");
    expect(afk.hasAttribute("disabled")).toBe(true);
    expect(within(afk.closest(".nav-switch") as HTMLElement).getByText("AFK is locked by policy.")).toBeTruthy();
    expect(afk.closest(".nav-switch")?.previousElementSibling?.classList.contains("nav-backup")).toBe(true);
  });

  it("shows the daemon's disabled reasons, and never sends an unsupported or disabled action", async () => {
    const disabled = JSON.parse(goldenText("command-afk-disabled.json")) as Record<string, unknown>;
    const daemon = daemonFor({ "/api/v1/state": () => json({ ...disabled, actions: [
      { id: "pause", label: "Pause", enabled: false, reason: "Pause is not available." },
      { id: "unknown", label: "Custom", enabled: true, reason: null }
    ] }) });
    renderPage("/overview");
    const controls = await screen.findByLabelText("Global controls");
    expect(await within(controls).findByText("Pause is not available.")).toBeTruthy();
    expect(within(controls).getByText("Unsupported action.")).toBeTruthy();
    expect(within(controls).getByRole("button", { name: "Pause" }).hasAttribute("disabled")).toBe(true);
    expect(within(controls).getByRole("button", { name: "Custom" }).hasAttribute("disabled")).toBe(true);
    expect(daemon.posts()).toEqual([]);
  });

  it("shows a refused command's problems without changing the action state", async () => {
    daemonFor({ "/api/v1/pause": refusal("Command denied", ["daemon reason"]) });
    const user = userEvent.setup();
    renderPage("/overview");
    const controls = await screen.findByLabelText("Global controls");
    const pause = await within(controls).findByRole("button", { name: "Pause everything" });
    await waitFor(() => expect(pause.hasAttribute("disabled")).toBe(false));
    const originalClass = pause.className;
    await user.click(pause);
    expect(await within(controls).findByText("Command denied")).toBeTruthy();
    expect(within(controls).getByText("daemon reason")).toBeTruthy();
    expect(pause.className).toBe(originalClass);
  });

  it("renders version mismatch with a user-driven Reload control", async () => {
    daemonFor({ "/api/v1/version": () => json({ apiVersion: 2, version: "next" }) });
    const reload = vi.fn();
    render(<AppProviders reloadDocument={reload}><MemoryRouter initialEntries={["/work"]}><AppRoutes /></MemoryRouter></AppProviders>);
    const reloadButton = await screen.findByRole("button", { name: "Reload" });
    expect(screen.getByText("API version changed — reload required")).toBeTruthy();
    expect(reload).not.toHaveBeenCalled();
    await userEvent.setup().click(reloadButton);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("shows offline state, retains loaded content, and shows no in-page notice through navigation", async () => {
    daemonFor();
    const user = userEvent.setup();
    renderRoutes("/overview");
    await screen.findByText(/^Good (morning|afternoon|evening)\.$/);
    AppSocket.instances[0]!.send({ type: "notification", title: "Daemon notice", body: "One <message>" });
    expect(screen.queryByText("Daemon notice")).toBeNull();
    expect(screen.queryByRole("region", { name: "Notifications" })).toBeNull();
    await user.click(screen.getByRole("link", { name: "Work" }));
    await screen.findByRole("heading", { name: "Work" });
    expect(await screen.findByText(/^Showing \d+ runs$/)).toBeTruthy();
    expect(AppSocket.instances).toHaveLength(1);

    Object.defineProperty(window.navigator, "onLine", { configurable: true, value: false });
    window.dispatchEvent(new Event("offline"));
    expect(await screen.findByText("Offline — reconnecting")).toBeTruthy();
    expect(screen.getByText(/^Showing \d+ runs$/)).toBeTruthy();
    expect(within(screen.getByLabelText("Global controls")).getByRole("button", { name: "Fetch" }).hasAttribute("disabled")).toBe(true);
  });

  it("persists navigation collapse and width changes, including the double-click reset", async () => {
    localStorage.clear();
    daemonFor();
    const user = userEvent.setup();
    const first = renderPage("/overview");
    await screen.findByRole("heading", { level: 1, name: "Overview" });
    await user.click(screen.getByRole("button", { name: "Hide navigation" }));
    await screen.findByRole("button", { name: "Show navigation" });
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey) ?? "{}")).toMatchObject({ navigationOpen: false });
    await user.click(screen.getByRole("button", { name: "Show navigation" }));
    const handle = screen.getByRole("separator", { name: "Resize navigation" });
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(handle.getAttribute("aria-valuenow")).toBe("230");
    fireEvent.doubleClick(handle);
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey) ?? "{}")).toMatchObject({ navigationWidth: defaultPreferences.navigationWidth });
    first.unmount();
    renderPage("/overview");
    expect(await screen.findByRole("button", { name: "Hide navigation" })).toBeTruthy();
  });
});

/** Renders the whole app at `path`: the page's own daemon answers, with the router probes left out. */
function renderPage(path: string) {
  return render(<TestApp initialEntries={[path]} />);
}
