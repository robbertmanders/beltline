import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defaultPreferences, preferencesStorageKey } from "../../state/preferences";
import { AppSocket } from "../../test/setup";
import { firstLive, mockDaemon, nameInput, renderAt, selectBelt, type DaemonRequest, type Handler, beltsList } from "./beltsTestKit";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";

function assistantRoutes(): Record<string, Handler | string> {
  let resource = JSON.parse(goldenText("assistant.json")) as Record<string, any>;
  return {
    "/api/v1/assistant": () => json(resource),
    "/api/v1/models/pi": goldenText("models.json"),
    "POST /api/v1/assistant/configuration": request => {
      resource = { ...resource, configuration: request.body };
      return json(resource);
    },
    "POST /api/v1/assistant/start": () => {
      resource = {
        ...resource,
        running: true,
        command: null,
        actions: [
          { id: "configuration", enabled: false, label: "Change configuration", reason: "Stop the assistant before changing its configuration." },
          { id: "start", enabled: false, label: "Start", reason: "The assistant is already running." },
          { id: "stop", enabled: true, label: "Stop", reason: null }
        ]
      };
      return json(resource);
    },
    "POST /api/v1/assistant/stop": () => {
      resource = JSON.parse(goldenText("assistant.json")) as Record<string, any>;
      return json(resource);
    }
  };
}

function savedBelt(request: DaemonRequest) {
  const belt = request.body.belt as Record<string, unknown>;
  return { ...firstLive, belt: { ...firstLive.belt, ...belt, version: firstLive.belt.version + 1 } };
}

beforeEach(() => {
  localStorage.clear();
  vi.unstubAllGlobals();
  // Unstubbing drops the setup's event socket too; without it the app never connects and every command is gated.
  vi.stubGlobal("WebSocket", AppSocket);
  AppSocket.instances = [];
});

describe("Belts assistant entry", () => {
  it("toggles Agent and Hide, persists width/visibility on remount, and keeps belt selection and its draft", async () => {
    const user = userEvent.setup();
    localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, assistantOpen: false, assistantWidth: 640 }));
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), ...assistantRoutes() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstLive.belt.name });
    await selectBelt(user, "Docs fixes");
    await user.clear(nameInput());
    await user.type(nameInput(), "Docs fixes draft");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const opening = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(opening).getByRole("button", { name: "Open without saving" }));
    const panel = await screen.findByRole("complementary", { name: "Assistant" });
    const divider = within(panel).getByRole("separator", { name: "Resize Assistant" });
    expect(divider.getAttribute("aria-valuenow")).toBe("640");

    fireEvent.keyDown(divider, { key: "ArrowLeft" });
    expect(divider.getAttribute("aria-valuenow")).toBe("656");
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).assistantWidth).toBe(656);
    await user.click(within(panel).getByRole("button", { name: "Hide ›" }));
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();
    expect(nameInput().value).toBe("Docs fixes draft");
    expect(screen.getByLabelText("Location").textContent).toContain("belt=docs");

    // Agent opens it again, and that visibility is what the remount below keeps.
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    await user.click(within(await screen.findByRole("dialog", { name: "Save your changes first?" })).getByRole("button", { name: "Open without saving" }));
    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeTruthy();

    // Navigating away remounts both pages while the app-level draft and browser preference remain alive.
    await user.click(screen.getByRole("button", { name: "Go to agents" }));
    await screen.findByRole("heading", { level: 1, name: "Agents" });
    await user.click(screen.getByRole("button", { name: "Back" }));
    await screen.findByRole("heading", { level: 1, name: "Belts" });
    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect(nameInput().value).toBe("Docs fixes draft");
    expect(screen.getByLabelText("Location").textContent).toContain("belt=docs");
    expect(screen.getByRole("separator", { name: "Resize Assistant" }).getAttribute("aria-valuenow")).toBe("656");
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).assistantOpen).toBe(true);
    expect(daemon.posts("/api/v1/assistant/stop")).toHaveLength(0);
  });

  it("offers Cancel, Open without saving and Save and open for dirty editor changes", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), ...assistantRoutes() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstLive.belt.name });
    await user.type(nameInput(), " draft");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const firstSheet = await screen.findByRole("dialog", { name: "Save your changes first?" });
    expect(within(firstSheet).getByText(/changes your belts and agents directly.*overwrite or remove what you are editing/i)).toBeTruthy();
    await user.click(within(firstSheet).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();
    expect(nameInput().value).toBe(`${firstLive.belt.name} draft`);
    expect(daemon.posts("/api/v1/belts/save")).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const openSheet = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(openSheet).getByRole("button", { name: "Open without saving" }));
    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect(nameInput().value).toBe(`${firstLive.belt.name} draft`);
  });

  it("opens only after Save and open succeeds", async () => {
    const user = userEvent.setup();
    let records = [firstLive];
    const daemon = mockDaemon({
      "/api/v1/belts": () => json(beltsList(records)),
      ...assistantRoutes(),
      "POST /api/v1/belts/save": request => {
        const resource = savedBelt(request);
        records = records.map(record => record.belt.id === resource.belt.id ? resource : record);
        return json({ resource, warnings: [] });
      }
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstLive.belt.name });
    await user.type(nameInput(), " saved");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const sheet = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(sheet).getByRole("button", { name: "Save and open" }));
    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect(daemon.posts("/api/v1/belts/save")).toHaveLength(1);
    expect(daemon.posts("/api/v1/belts/save")[0]!.body.belt.name).toBe(`${firstLive.belt.name} saved`);
  });

  it("requires separate confirmation before Start when the editor is dirty", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), ...assistantRoutes() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstLive.belt.name });
    await user.type(nameInput(), " unsaved");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const openSheet = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(openSheet).getByRole("button", { name: "Open without saving" }));
    const assistant = await screen.findByRole("complementary", { name: "Assistant" });
    await user.type(within(assistant).getByRole("textbox", { name: "Request" }), "Add a belt");

    await user.click(within(assistant).getByRole("button", { name: "Start" }));
    const warning = await screen.findByRole("dialog", { name: "Save your changes first?" });
    expect(within(warning).getByText(/changes your belts and agents directly.*overwrite or remove what you are editing/i)).toBeTruthy();
    await user.click(within(warning).getByRole("button", { name: "Cancel" }));
    expect(daemon.posts("/api/v1/assistant/start")).toHaveLength(0);
    expect(nameInput().value).toBe(`${firstLive.belt.name} unsaved`);

    await user.click(within(assistant).getByRole("button", { name: "Start" }));
    const confirm = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(confirm).getByRole("button", { name: "Start anyway" }));
    await waitFor(() => expect(daemon.posts("/api/v1/assistant/start")).toHaveLength(1));
    expect(daemon.posts("/api/v1/assistant/start")[0]!.body).toEqual({ request: "Add a belt" });
    expect(nameInput().value).toBe(`${firstLive.belt.name} unsaved`);
  });
});
