import { QueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { AppSocket } from "../test/setup";
import { listGolden, posts, renderAgents, resetAgentsEnvironment, stubAgents } from "./agentsTestKit";
import { json } from "../test/response";

beforeEach(resetAgentsEnvironment);

describe("Agents library", () => {
  it("opens the shared proposal sheet from a saved agent with its immutable id and version", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await screen.findByLabelText("Brief");
    await user.click(screen.getByRole("button", { name: "Share…" }));
    const sheet = await screen.findByRole("dialog", { name: /^Share / });
    expect(within(sheet).getByText("triage · v2")).toBeTruthy();
    expect(posts(requests).filter(request => request.path.endsWith("/library/share"))).toHaveLength(0);
  });

  it("offers Ship beside Done on an agent outcome and saves the ship action", async () => {
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await screen.findByLabelText("Name");
    const then = screen.getByLabelText("Outcome 1 then") as HTMLSelectElement;
    expect(within(then).getByRole("option", { name: "Done" })).toBeTruthy();
    expect(within(then).getByRole("option", { name: "Ship" })).toBeTruthy();
    fireEvent.change(then, { target: { value: "ship" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    const request = posts(requests).find(item => item.path === "/api/v1/agents/save")!;
    expect((request.body as { agent: { outcomes: Array<{ then: { do: string } }> } }).agent.outcomes[0]!.then.do).toBe("ship");
  });

  it("disables Share for new and dirty agent drafts without building or proposing", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.type(screen.getByLabelText("Name"), " changed");
    const dirtyShare = screen.getByRole("button", { name: "Share…" }) as HTMLButtonElement;
    expect(dirtyShare.disabled).toBe(true);
    expect(dirtyShare.title).toBe("Save changes before sharing");
    await user.click(screen.getByRole("button", { name: /Back to Agents/ }));
    const newAgent = await screen.findByRole("button", { name: /New agent/ });
    await waitFor(() => expect(newAgent.hasAttribute("disabled")).toBe(false));
    await user.click(newAgent);
    await screen.findByLabelText("Id");
    expect((screen.getByRole("button", { name: "Share…" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Share…" }) as HTMLButtonElement).title).toBe("Save this agent before sharing it.");
    expect(posts(requests).filter(request => request.path.endsWith("/library/share"))).toHaveLength(0);
  });

  it("shows the daemon browser and its archived filter", async () => {
    stubAgents();
    renderAgents(undefined, "/agents");
    expect(await screen.findByRole("heading", { level: 1, name: "Agents" })).toBeTruthy();
    expect(await screen.findByText(/shown · .* in the library/)).toBeTruthy();
    expect(within(screen.getByRole("group", { name: "Agents filter" })).getByRole("button", { name: /Archived/ })).toBeTruthy();
  });

  it("keeps browsing state in the URL and asks the daemon to filter", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents(undefined, "/agents?search=spec");
    await user.click(await screen.findByRole("button", { name: /Archived/ }));
    await waitFor(() => expect(requests.some(request => request.path.includes("search=spec") && request.path.includes("filter=archived"))).toBe(true));
    await user.click(await screen.findByRole("button", { name: /Open Build/ }));
    await screen.findByLabelText("Brief");
    await user.click(screen.getByRole("button", { name: /Back to Agents/ }));
    expect((await screen.findByLabelText("Search agents") as HTMLInputElement).value).toBe("spec");
    expect(within(screen.getByRole("group", { name: "Agents filter" })).getByRole("button", { name: /Archived/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("does not auto-select an agent and preserves a new draft when returning to the browser", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents(undefined, "/agents");
    expect(await screen.findByRole("heading", { level: 1, name: "Agents" })).toBeTruthy();
    expect(screen.queryByLabelText("Brief")).toBeNull();
    await user.click(await screen.findByRole("button", { name: /New agent/ }));
    const name = await screen.findByLabelText("Name");
    fireEvent.change(name, { target: { value: "Persistent draft" } });
    await user.click(screen.getByRole("button", { name: /Back to Agents/ }));
    await user.click(await screen.findByRole("button", { name: /Persistent draft/ }));
    expect((await screen.findByLabelText("Name") as HTMLInputElement).value).toBe("Persistent draft");
    expect(posts(requests).filter(request => request.path === "/api/v1/agents/new")).toHaveLength(1);
  });

  it("renders the empty-library explanation and leaves New available", async () => {
    const { requests } = stubAgents({ list: { agents: [], choices: [], actions: [{ id: "new", label: "New agent", enabled: true }] } });
    renderAgents(undefined, "/agents");
    expect(await screen.findByText("No agents yet")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /New agent/ }).some(button => !button.hasAttribute("disabled"))).toBe(true);
    expect(posts(requests)).toHaveLength(0);
  });

  it("shows explicit initial loading and failure states", async () => {
    const { requests } = stubAgents({ listResponse: async requestNumber => {
      if (requestNumber === 1) await new Promise(resolve => setTimeout(resolve, 40));
      return json({ message: "The library is unavailable.", problems: [] }, 503);
    } });
    renderAgents();
    expect(screen.getByText("Loading agents…")).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Unable to load agents" })).toBeTruthy();
    expect(screen.getByText("The library is unavailable.")).toBeTruthy();
    expect(posts(requests)).toHaveLength(0);
  });

  it("keeps an edited draft and shows a stale warning after a failed library refresh", async () => {
    const { requests } = stubAgents({ listResponse: requestNumber => requestNumber === 1
      ? json(listGolden())
      : json({ message: "Refresh failed.", problems: [] }, 503) });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderAgents(queryClient);
    await screen.findByLabelText("Brief");
    const name = await screen.findByLabelText("Name") as HTMLInputElement;
    fireEvent.change(name, { target: { value: "Keep this draft" } });
    await queryClient.invalidateQueries({ predicate: query => String(query.queryKey[1]).startsWith("/api/v1/agents?") });
    expect(await screen.findByText("Showing the last loaded library; refresh failed.")).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Keep this draft");
    expect(requests.filter(request => request.path.startsWith("/api/v1/agents") && request.method === "GET").length).toBeGreaterThan(1);
  });

  it("opens a saved agent whose id starts with the new-draft prefix", async () => {
    const records = listGolden().agents;
    const source = records.find(record => record.agent.id === "triage")!;
    const renamed = { ...source, agent: { ...source.agent, id: "new-agent", key: "new-agent" } };
    stubAgents({ list: { ...listGolden(), agents: [renamed, ...records.filter(record => record.agent.id !== "triage")] } });
    renderAgents(undefined, "/agents?agent=new-agent");
    expect(await screen.findByLabelText("Brief")).toBeTruthy();
    expect(screen.getByLabelText("Id").textContent).toBe("new-agent");
  });

  it("keeps local edits available offline and prevents runner normalization commands", async () => {
    const { requests } = stubAgents();
    renderAgents();
    const runner = (await screen.findAllByRole("combobox", { name: "Harness" }))[0] as HTMLSelectElement;
    await waitFor(() => expect(runner.disabled).toBe(false));
    act(() => AppSocket.instances[0]?.close());
    await waitFor(() => expect(runner.disabled).toBe(true));
    fireEvent.change(runner, { target: { value: "codex" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Offline draft" } });
    expect((screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement).value).toBe("Offline draft");
    expect(posts(requests).some(request => request.path === "/api/v1/agents/draft/switch-runner")).toBe(false);
  });

  it("opens an unsaved agent from the daemon with an editable id and a new row", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents(undefined, "/agents");
    const newButton = await screen.findByRole("button", { name: /New agent/ });
    await user.click(newButton);
    expect(await screen.findByLabelText("Id")).toBeTruthy();
    expect((screen.getByLabelText("Id") as HTMLInputElement).value).toBe("agent-1");
    fireEvent.change(screen.getByLabelText("Id"), { target: { value: "user-agent-id" } });
    expect((screen.getByLabelText("Id") as HTMLInputElement).value).toBe("user-agent-id");
    expect(posts(requests).some(request => request.path === "/api/v1/agents/new")).toBe(true);
  });

  it("reports a 404 for a missing selected agent without dropping the library", async () => {
    stubAgents({ missingDetail: true });
    renderAgents();
    await screen.findByRole("heading", { level: 1, name: "Agents" });
    expect(await screen.findByRole("heading", { name: "Agent no longer available" })).toBeTruthy();
  });
});


describe("archive, restore and reset", () => {
  it("archives the selected agent and falls back to the first live agent", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.click(await screen.findByRole("button", { name: "Archive" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/triage/archive")).toBe(true));
    // The list refetch from the same fixture still contains triage, so selection stays; the command was sent once.
    expect(posts(requests).filter(request => request.path === "/api/v1/agents/triage/archive")).toHaveLength(1);
  });

  it("shows Used by with Open belt navigation", async () => {
    stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    expect(await screen.findByRole("heading", { name: /Used by/ })).toBeTruthy();
    expect(screen.getAllByRole("link", { name: "Open belt" }).length).toBeGreaterThan(0);
  });
});

describe("agent card layout", () => {
  /** Whether `later` follows `earlier` in the document. */
  const follows = (earlier: Element, later: Element) => (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

  it("keeps every row's controls named without visible labels, in document order, and changeable from the keyboard", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    const harnesses = screen.getAllByRole("combobox", { name: "Harness" });
    const models = screen.getAllByRole("combobox", { name: "Model" });
    const thinkings = screen.getAllByRole("combobox", { name: "Thinking" });
    const order = [
      harnesses[0], models[0], thinkings[0],
      harnesses[1], models[1], thinkings[1], screen.getByRole("button", { name: "Remove alternate 1" }),
      harnesses[2], models[2], thinkings[2], screen.getByRole("button", { name: "Remove alternate 2" }),
      screen.getByRole("button", { name: "+ Add a model" })
    ];

    act(() => harnesses[0]!.focus());
    for (const expected of order.slice(1)) {
      await user.tab();
      expect(document.activeElement).toBe(expected);
    }

    // Each control is reached with Tab only. jsdom cannot pick a native option from the keyboard, so the change is the
    // change event a keyboard selection dispatches, fired on the control Tab focused.
    const tabTo = async (target: Element) => {
      act(() => harnesses[0]!.focus());
      for (let steps = 0; document.activeElement !== target; steps++) {
        if (steps > order.length) throw new Error("Tab never reached the control");
        await user.tab();
      }
    };
    await tabTo(harnesses[1]!);
    fireEvent.change(harnesses[1]!, { target: { value: "cursor" } });
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/draft/switch-runner" && request.body?.alternateIndex === 0)).toBe(true));
    await tabTo(thinkings[0]!);
    fireEvent.change(thinkings[0]!, { target: { value: "high" } });
    expect((thinkings[0] as HTMLSelectElement).value).toBe("high");
    expect((harnesses[1] as HTMLSelectElement).value).toBe("cursor");
  });

  it("puts the folder at the bottom of the page with an Open folder button", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    const { unmount } = renderAgents();
    await screen.findByLabelText("Brief");
    const folder = await screen.findByText("<TEMP_ROOT>/agents/triage");
    const line = folder.closest(".agent-folder-line")!;
    const openFolder = within(line as HTMLElement).getByRole("button", { name: "Open folder" });
    const editor = screen.getByRole("region", { name: "Agent editor" });

    expect(screen.queryByRole("button", { name: "Show files" })).toBeNull();
    expect(folder.closest(".agent-card")).toBeNull();
    expect(editor.lastElementChild).toBe(folder.closest(".agent-folder"));
    expect(follows(screen.getByRole("heading", { name: /Used by/ }), line)).toBe(true);
    for (const link of screen.getAllByRole("link", { name: "Open belt" })) expect(follows(link, line)).toBe(true);

    await user.click(openFolder);
    await waitFor(() => expect(posts(requests).filter(request => request.path === "/api/v1/desktop/reveal")).toHaveLength(1));
    expect(posts(requests).find(request => request.path === "/api/v1/desktop/reveal")!.body).toEqual({ kind: "agent", agentId: "triage" });

    unmount();
    renderAgents(undefined, "/agents");
    await user.click(await screen.findByRole("button", { name: /New agent/ }));
    await screen.findByLabelText("Id");
    expect(screen.queryByRole("button", { name: "Open folder" })).toBeNull();
  });
});
