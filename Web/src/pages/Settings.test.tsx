import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyStoredAppearance } from "../state/theme";
import { json } from "../test/response";
import { goldenText } from "../test/golden";
import { fixtureRoute, refusal, renderPage, stubDaemon, type DaemonRequest, type Handler } from "../test/daemon";

/** The daemon for Settings: its golden reads, with the routes a test changes. */
function daemonFor(routes: Record<string, Handler> = {}) {
  const daemon = stubDaemon({
    "/api/v1/settings": fixtureRoute("settings.json"),
    "/api/v1/models/pi": fixtureRoute("models.json"),
    "/api/v1/models/pi/allow": fixtureRoute("models.json"),
    "/api/v1/tools": fixtureRoute("tools.json"),
    ...routes
  });
  return daemon;
}

async function renderSettings(routes: Record<string, Handler> = {}) {
  const daemon = daemonFor(routes);
  renderPage("/settings");
  await screen.findByRole("heading", { level: 1, name: "Settings" });
  await waitFor(() => expect(screen.queryByText("Settings loaded")).toBeTruthy());
  return daemon;
}

const notifyCheckbox = () => screen.getByRole("checkbox", { name: "Notify me when a run needs me" }) as HTMLInputElement;
const settingsWithNotifications = (notifications: boolean) => JSON.stringify({ ...JSON.parse(goldenText("settings.json")) as Record<string, unknown>, notifications });

/** Settings whose save is kept by the daemon, so the next read confirms it; `rejected` refuses the save instead. */
function notificationRoutes(confirmed: boolean, rejected = false): Record<string, Handler> {
  let saved = confirmed;
  return {
    "/api/v1/settings": () => json(settingsWithNotifications(saved)),
    "/api/v1/settings/patch": (request: DaemonRequest) => {
      if (rejected) return json({ message: "Settings rejected", problems: ["Choose an allowed model"] }, 422);
      saved = (request.body as { notifications: boolean }).notifications;
      return json(settingsWithNotifications(saved));
    }
  };
}

describe("Settings cards", () => {
  it("renders each tool row's state from the daemon", async () => {
    const tools = JSON.stringify({ tools: [
      { name: "gh", purpose: "GitHub", ok: true, detail: "installed", optional: false },
      { name: "tmux", purpose: "sessions", ok: false, detail: "missing", optional: false },
      { name: "Ghostty", purpose: "terminal", ok: false, detail: "not installed", optional: true }
    ], tmuxSocket: "beltline" });
    await renderSettings({ "/api/v1/tools": () => json(tools) });
    await waitFor(() => expect(screen.getAllByRole("listitem").some(row => row.textContent?.includes("tmux"))).toBe(true));
    const rows = screen.getAllByRole("listitem");
    expect(rows.find(row => row.textContent?.includes("tmux"))!.getAttribute("data-state")).toBe("missing-required");
    expect(rows.find(row => row.textContent?.includes("Ghostty"))!.getAttribute("data-state")).toBe("missing-optional");
    expect(rows.find(row => row.textContent?.includes("installed"))!.getAttribute("data-state")).toBe("available");
  });

  it("saves the library repositories as one patch, with an edited, a removed and an added row", async () => {
    const daemon = await renderSettings({ "/api/v1/settings/patch": fixtureRoute("settings.json") });
    const user = userEvent.setup();
    const card = screen.getByRole("region", { name: "Library repositories" });
    const [shared] = within(card).getAllByLabelText("Library repository") as HTMLInputElement[];
    expect(shared!.value).toBe("example/shared");
    await user.clear(shared!);
    await user.type(shared!, "owner/repo");
    await user.click(within(card).getAllByRole("button", { name: "Remove" })[1]!);
    await user.click(within(card).getByRole("button", { name: "Add repository" }));
    await user.type(within(card).getAllByLabelText("Library repository")[1] as HTMLInputElement, "owner/backup");
    await user.click(within(card).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(daemon.posts("/api/v1/settings/patch")).toHaveLength(1));
    expect(daemon.posts("/api/v1/settings/patch")[0]!.body).toEqual({ libraryRepositories: [
      { repository: "owner/repo", path: "", role: "shared", publish: "push" },
      { repository: "owner/backup", path: "", role: "shared", publish: "pullRequest" }
    ] });
  });

  it("shows a refused library save inline and keeps the unsent rows", async () => {
    await renderSettings({ "/api/v1/settings/patch": refusal("Invalid repository", ["owner/repo looks wrong"]) });
    const user = userEvent.setup();
    const field = screen.getAllByLabelText("Library repository")[0] as HTMLInputElement;
    await user.clear(field);
    await user.type(field, "bad");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Invalid repository")).toBeTruthy();
    expect(screen.getByText("owner/repo looks wrong")).toBeTruthy();
    expect((screen.getAllByLabelText("Library repository")[0] as HTMLInputElement).value).toBe("bad");
  });

  it("starts Allowed models collapsed, and an allow change sends the model's id and the new state", async () => {
    const daemon = await renderSettings();
    expect(screen.queryByLabelText("Search models")).toBeNull();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Allowed models" }));
    expect(await screen.findByLabelText("Search models")).toBeTruthy();
    const fixtureRow = screen.getByText("Fixture model (demo)").closest("li")!;
    const fixtureCheckbox = within(fixtureRow).getByRole("checkbox", { name: "Fixture model (demo)" }) as HTMLInputElement;
    expect(fixtureCheckbox.checked).toBe(true);
    await user.click(fixtureCheckbox);
    await waitFor(() => expect(daemon.posts("/allow")).toHaveLength(1));
    expect(daemon.posts("/allow")[0]!.body).toEqual({ model: "fixture-model", allowed: false });
  });

  it("explains macOS notices without browser permission controls, and never prompts", async () => {
    const requestPermission = vi.fn(async () => "default" as NotificationPermission);
    const construct = vi.fn();
    class DefaultNotification {
      static permission = "default";
      static requestPermission = requestPermission;
      constructor(title: string, options?: NotificationOptions) { construct(title, options); }
    }
    vi.stubGlobal("Notification", DefaultNotification);
    const daemon = await renderSettings({ "/api/v1/settings/patch": fixtureRoute("settings.json") });
    expect(screen.getByText("Send macOS notifications when a run needs you.")).toBeTruthy();
    expect(screen.queryByText(/Permission:/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Allow notifications" })).toBeNull();
    await userEvent.setup().click(notifyCheckbox());
    await waitFor(() => expect(daemon.posts("/settings/patch")).toHaveLength(1));
    expect(requestPermission).not.toHaveBeenCalled();
    expect(construct).not.toHaveBeenCalled();
  });

  it("saves a notification change as an exact patch and shows the confirmed value, or the refusal with the checkbox kept", async () => {
    const user = userEvent.setup();
    const saved = await renderSettings(notificationRoutes(false));
    expect(notifyCheckbox().checked).toBe(false);
    await user.click(notifyCheckbox());
    await waitFor(() => expect(notifyCheckbox().checked).toBe(true));
    expect(saved.posts("/settings/patch")[0]!.body).toEqual({ notifications: true });
    cleanup();

    await renderSettings(notificationRoutes(true, true));
    await user.click(notifyCheckbox());
    expect(await screen.findByText("Settings rejected")).toBeTruthy();
    expect(screen.getByText("Choose an allowed model")).toBeTruthy();
    expect(notifyCheckbox().checked).toBe(true);
  });
});

describe("Appearance in General", () => {
  const appearanceGroup = () => screen.getByRole("group", { name: "Appearance" });
  const pressed = () => within(appearanceGroup()).getAllByRole("button").filter(button => button.getAttribute("aria-pressed") === "true").map(button => button.textContent);

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });
  afterEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("offers Light and Dark with Light pressed first and the browser-only helper text", async () => {
    await renderSettings();
    expect(within(appearanceGroup()).getAllByRole("button").map(button => button.textContent)).toEqual(["Light", "Dark"]);
    expect(pressed()).toEqual(["Light"]);
    expect(screen.getByText("Applies to this browser.")).toBeTruthy();
  });

  it("switches the whole app by click and by keyboard, sending no daemon command", async () => {
    const daemon = await renderSettings();
    const user = userEvent.setup();
    await user.click(within(appearanceGroup()).getByRole("button", { name: "Dark" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    within(appearanceGroup()).getByRole("button", { name: "Light" }).focus();
    await user.keyboard("{Enter}");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(pressed()).toEqual(["Light"]);
    expect(daemon.posts()).toEqual([]);
  });

  it("keeps a Dark choice through navigation, a reload and a remount in the same browser", async () => {
    const user = userEvent.setup();
    await renderSettings();
    await user.click(within(appearanceGroup()).getByRole("button", { name: "Dark" }));
    await user.click(screen.getByRole("link", { name: "Beltline" }));
    await user.click(screen.getByRole("link", { name: "Settings" }));
    await screen.findByRole("heading", { level: 1, name: "Settings" });
    expect(pressed()).toEqual(["Dark"]);
    cleanup();

    document.documentElement.removeAttribute("data-theme");
    applyStoredAppearance();
    await renderSettings();
    expect(pressed()).toEqual(["Dark"]);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("applies the selected appearance for the visit when storage refuses writes", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    await renderSettings();
    await userEvent.setup().click(within(appearanceGroup()).getByRole("button", { name: "Dark" }));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
