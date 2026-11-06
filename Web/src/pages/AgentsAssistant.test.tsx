import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { defaultPreferences, preferencesStorageKey } from "../state/preferences";
import { posts, renderAgents, resetAgentsEnvironment, stubAgents } from "./agentsTestKit";
import { json } from "../test/response";

beforeEach(resetAgentsEnvironment);

describe("Agents assistant sidebar", () => {
  it("opens and hides the shared assistant while retaining the selected agent and unsent Name draft", async () => {
    const user = userEvent.setup();
    localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, assistantOpen: false, assistantWidth: 620 }));
    stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.type(await screen.findByLabelText("Name"), " draft");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const opening = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(opening).getByRole("button", { name: "Open without saving" }));
    const assistant = await screen.findByRole("complementary", { name: "Assistant" });
    expect(within(assistant).getByRole("textbox", { name: "Request" })).toBeTruthy();
    expect((within(assistant).getByRole("separator", { name: "Resize Assistant" })).getAttribute("aria-valuenow")).toBe("620");
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage draft");

    await user.click(within(assistant).getByRole("button", { name: "Hide ›" }));
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage draft");
    expect(screen.getByRole("heading", { name: "Triage draft" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const reopening = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(reopening).getByRole("button", { name: "Open without saving" }));
    expect(await screen.findByRole("complementary", { name: "Assistant" })).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage draft");
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).assistantOpen).toBe(true);
  });

  it("confirms dirty Start separately, sends no POST on cancel and starts only after Start anyway", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.type(await screen.findByLabelText("Name"), " draft");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const opening = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(opening).getByRole("button", { name: "Open without saving" }));
    const assistant = await screen.findByRole("complementary", { name: "Assistant" });
    await user.type(within(assistant).getByRole("textbox", { name: "Request" }), "Adjust the selected agent");
    await user.click(within(assistant).getByRole("button", { name: "Start" }));
    const warning = await screen.findByRole("dialog", { name: "Save your changes first?" });
    expect(within(warning).getByText(/changes your belts and agents directly.*overwrite or remove what you are editing/i)).toBeTruthy();
    await user.click(within(warning).getByRole("button", { name: "Cancel" }));
    expect(requests.filter(request => request.path === "/api/v1/assistant/start")).toHaveLength(0);
    await user.click(within(assistant).getByRole("button", { name: "Start" }));
    const confirm = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(confirm).getByRole("button", { name: "Start anyway" }));
    await screen.findByText("Simulated conversation. No real agent is launched in demo mode.");
    expect(requests.filter(request => request.path === "/api/v1/assistant/start").map(request => request.body)).toEqual([{ request: "Adjust the selected agent" }]);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage draft");
  });

  it("keeps the assistant closed and the agent draft when Save and open is refused", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents({ save: () => json({ message: "Agent save refused.", problems: ["The name is not available."] }, 422) });
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.type(await screen.findByLabelText("Name"), " rejected");
    await user.click(screen.getByRole("button", { name: "Ask the assistant" }));
    const opening = await screen.findByRole("dialog", { name: "Save your changes first?" });
    await user.click(within(opening).getByRole("button", { name: "Save and open" }));
    expect(await screen.findByText("Agent save refused.")).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Assistant" })).toBeNull();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage rejected");
    expect(posts(requests).filter(request => request.path === "/api/v1/agents/save")).toHaveLength(1);
  });

});
