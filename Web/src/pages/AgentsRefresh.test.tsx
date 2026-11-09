import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { history, libraryChanged } from "../test/daemon";
import { agentSaves, configure, detailFor, listGolden, renderAgents, resetAgentsEnvironment, stubAgents } from "./agentsTestKit";

beforeEach(resetAgentsEnvironment);

describe("saved agent refresh", () => {
  it("reads a configured agent again when it is reopened in the mounted page", async () => {
    const list = listGolden();
    const { requests } = stubAgents({ list });
    renderAgents();
    await screen.findByLabelText("Brief");
    configure(list, "triage", "Triage, configured");
    fireEvent.click(screen.getByRole("button", { name: "Go to agents" }));
    fireEvent.click(await screen.findByRole("button", { name: "Open Triage, configured" }));
    await waitFor(() => expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage, configured"));
    expect(requests.filter(request => request.method === "GET" && request.path === "/api/v1/agents/triage").length).toBeGreaterThanOrEqual(2);
    expect(agentSaves(requests)).toHaveLength(0);
  });

  it("refreshes a clean open agent when the library changes, and the agent's fields come from its detail", async () => {
    // The page's list response stays old: the daemon's detail for the open agent is the new version, and the editor must show it.
    const list = listGolden();
    const fresh = listGolden();
    configure(fresh, "triage", "Triage, from the detail");
    stubAgents({ list, detail: id => detailFor(fresh.agents.find(entry => entry.agent.id === id)!) });
    renderAgents();
    await screen.findByLabelText("Brief");
    libraryChanged(2);
    await waitFor(() => expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Triage, from the detail"));
  });

  it("keeps unsent edits through a refresh and navigation, and Discard shows the newest saved agent", async () => {
    const user = userEvent.setup();
    const list = listGolden();
    const { requests } = stubAgents({ list });
    renderAgents();
    await screen.findByLabelText("Brief");
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Local name" } });
    configure(list, "triage", "Configured name");
    libraryChanged(2);
    await waitFor(() => expect(requests.filter(request => request.path === "/api/v1/agents/triage").length).toBeGreaterThanOrEqual(2));
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Local name");
    await history(user, "Go to agents");
    await user.click(await screen.findByRole("button", { name: "Local name Draft" }));
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Local name");
    await user.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Configured name"));
    expect(agentSaves(requests)).toHaveLength(0);
  });

  it("gives a draft that is back to its saved agent way to a newer saved version", async () => {
    const user = userEvent.setup();
    const list = listGolden();
    stubAgents({ list });
    renderAgents();
    await screen.findByLabelText("Brief");
    const name = screen.getByLabelText("Name") as HTMLInputElement;
    const saved = name.value;
    await user.type(name, "x");
    await user.type(name, "{backspace}");
    expect(name.value).toBe(saved);
    configure(list, "triage", "Configured after a revert");
    libraryChanged(2);
    await waitFor(() => expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Configured after a revert"));
  });

  it("shows the missing state for an agent whose detail is gone, with no draft", async () => {
    const list = listGolden();
    stubAgents({ list });
    renderAgents();
    await screen.findByLabelText("Brief");
    list.agents = list.agents.filter(entry => entry.agent.id !== "triage");
    libraryChanged(2);
    expect(await screen.findByRole("heading", { level: 2, name: "Agent no longer available" })).toBeTruthy();
  });

  it("keeps a missing agent's unsent edits with an explanation, and Discard leads to the missing state without a save", async () => {
    const user = userEvent.setup();
    const list = listGolden();
    const { requests } = stubAgents({ list });
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.type(screen.getByLabelText("Name"), " unsent");
    list.agents = list.agents.filter(entry => entry.agent.id !== "triage");
    libraryChanged(2);
    expect(await screen.findByRole("heading", { level: 2, name: "Agent no longer in the library" })).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toContain("unsent");
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(await screen.findByRole("heading", { level: 2, name: "Agent no longer available" })).toBeTruthy();
    expect(agentSaves(requests)).toHaveLength(0);
  });

  it("keeps the open agent and its edits when a refresh fails, with the error treatment and no missing explanation", async () => {
    let failing = false;
    const list = listGolden();
    stubAgents({ list, failing: () => failing });
    renderAgents();
    await screen.findByLabelText("Brief");
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Edited before the failure" } });
    failing = true;
    libraryChanged(2);
    expect(await screen.findByText("Showing the last loaded agent; refresh failed.")).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Edited before the failure");
    expect(screen.queryByText(/no longer in the library/)).toBeNull();
    expect(screen.queryByRole("heading", { name: "Agent no longer available" })).toBeNull();
  });
});
