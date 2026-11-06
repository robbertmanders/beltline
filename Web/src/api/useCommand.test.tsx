import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../app/App";
import { AppSocket } from "../test/setup";
import { useCommand, type CommandController, type CommandRequest } from "./useCommand";
import type { StateResponse } from "./wire.gen";
import { globalCommandRequest } from "./globalCommands";
import { json } from "../test/response";
import { goldenText } from "../test/golden";

type Handler = (init?: RequestInit) => Response | Promise<Response>;

/** A daemon that serves the shell's reads and answers commands with the handlers it is given. */
function stubDaemon(options: { state?: () => string; commands?: Record<string, Handler> } = {}) {
  const requests: Array<{ method: string; path: string; body?: unknown }> = [];
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input);
    const method = init?.method ?? "GET";
    requests.push({ method, path, body: init?.body === undefined ? undefined : JSON.parse(String(init.body)) });
    const command = options.commands?.[`${method} ${path}`];
    if (command) return command(init);
    if (path === "/api/v1/version") return json(goldenText("version.json"));
    if (path === "/api/v1/state") return json(options.state?.() ?? goldenText("state.json"));
    if (path === "/api/v1/views/overview") return json(goldenText("views-overview.json"));
    return json(goldenText("error-not-found.json"), 404);
  });
  vi.stubGlobal("fetch", fetcher);
  const posts = () => requests.filter(request => request.method === "POST");
  const gets = (path: string) => requests.filter(request => request.method === "GET" && request.path === path).length;
  return { requests, posts, gets };
}

function renderShell() {
  render(<AppProviders><MemoryRouter initialEntries={["/overview"]}><AppRoutes /></MemoryRouter></AppProviders>);
  return screen.findByLabelText("Global controls");
}

function Harness({ into }: { into: { current: CommandController | null } }) {
  const command = useCommand();
  into.current = command;
  return <span>{command.pending ? "pending" : "idle"}</span>;
}

const pauseRequest = (state: StateResponse): CommandRequest<unknown, StateResponse> => globalCommandRequest("pause", state);
const loadedState = JSON.parse(goldenText("state.json")) as StateResponse;

describe("global command requests", () => {
  it("sends the opposite of the loaded paused flag, of the AFK setting, and an empty Check now body", () => {
    expect(pauseRequest(loadedState).body).toEqual({ paused: true });
    expect(pauseRequest({ ...loadedState, globallyPaused: true }).body).toEqual({ paused: false });
    expect(globalCommandRequest("afk", loadedState).body).toEqual({ enabled: true });
    expect(globalCommandRequest("afk", { ...loadedState, settings: { ...loadedState.settings, afkEnabled: true } }).body).toEqual({ enabled: false });
    expect(globalCommandRequest("check-now", loadedState).body).toEqual({});
  });
  it("names the reads each command affects, and only those", () => {
    expect(globalCommandRequest("pause", loadedState).affects).toEqual(["state"]);
    expect(globalCommandRequest("afk", loadedState).affects).toEqual(["state", "settings"]);
    expect(globalCommandRequest("check-now", loadedState).affects).toEqual(["state", "intake"]);
  });
});

describe("useCommand", () => {
  it("refuses a second submission while one is in flight, so only one POST leaves", async () => {
    const release: { resolve?: (response: Response) => void } = {};
    stubDaemon({ commands: { "POST /api/v1/check-now": () => new Promise<Response>(resolve => { release.resolve = resolve; }) } });
    const into: { current: CommandController | null } = { current: null };
    render(<AppProviders><Harness into={into} /></AppProviders>);
    const request: CommandRequest<unknown, StateResponse> = { ...globalCommandRequest("check-now", loadedState) };
    let first = false;
    let second = true;
    act(() => {
      first = into.current!.submit(request);
      second = into.current!.submit(request);
    });
    expect(first).toBe(true);
    expect(second).toBe(false);
    await waitFor(() => expect(screen.getByText("pending")).toBeTruthy());
    expect(release.resolve).toBeDefined();
    await act(async () => { release.resolve!(json(goldenText("command-check-now.json"))); });
    await waitFor(() => expect(screen.getByText("idle")).toBeTruthy());
  });

  it.each([200, 422])("keeps Pause and AFK usable during Fetch and releases Fetch after HTTP %s", async status => {
    const release: { resolve?: (response: Response) => void } = {};
    const { posts } = stubDaemon({ commands: {
      "POST /api/v1/check-now": () => new Promise<Response>(resolve => { release.resolve = resolve; }),
      "POST /api/v1/pause": () => json(goldenText("command-pause.json")),
      "POST /api/v1/afk": () => json(goldenText("command-afk.json"))
    } });
    const controls = await renderShell();
    const fetchButton = await within(controls).findByRole("button", { name: "Fetch" });
    await waitFor(() => expect(fetchButton.hasAttribute("disabled")).toBe(false));
    fireEvent.click(fetchButton);
    fireEvent.click(fetchButton);
    await waitFor(() => expect(release.resolve).toBeDefined());
    await waitFor(() => expect(within(controls).getAllByRole("button", { name: "Working…" })).toHaveLength(1));
    expect(fetchButton.hasAttribute("disabled")).toBe(true);
    const pause = within(controls).getByRole("button", { name: "Pause everything" });
    const afk = screen.getByRole("switch", { name: "Global AFK" });
    expect(pause.hasAttribute("disabled")).toBe(false);
    expect(afk.hasAttribute("disabled")).toBe(false);
    fireEvent.click(pause);
    fireEvent.click(afk);
    await waitFor(() => expect(posts().map(post => post.path)).toEqual(["/api/v1/check-now", "/api/v1/pause", "/api/v1/afk"]));
    // Finishing the other commands must not clear Fetch's pending state.
    await waitFor(() => expect(within(controls).getAllByRole("button", { name: "Working…" })).toHaveLength(1));
    expect(fetchButton.getAttribute("aria-label")).toBe("Working…");
    expect(fetchButton.hasAttribute("disabled")).toBe(true);
    await act(async () => { release.resolve!(status === 200
      ? json(goldenText("command-check-now.json"))
      : json('{"message":"Fetch denied","problems":[]}', status)); });
    await waitFor(() => expect(within(controls).getByRole("button", { name: "Fetch" }).hasAttribute("disabled")).toBe(false));
    if (status === 422) expect(await within(controls).findByText("Fetch denied")).toBeTruthy();
  });

  it("fires one pause POST when the control is activated twice", async () => {
    const { posts } = stubDaemon({ commands: { "POST /api/v1/pause": () => json(goldenText("command-pause.json")) } });
    const controls = await renderShell();
    const pause = await within(controls).findByRole("button", { name: "Pause everything" });
    await waitFor(() => expect(pause.hasAttribute("disabled")).toBe(false));
    fireEvent.click(pause);
    fireEvent.click(pause);
    await waitFor(() => expect(posts().length).toBe(1));
    expect(posts()[0]?.body).toEqual({ paused: true });
  });

  it("publishes the returned state without an event and refetches only the reads it affects", async () => {
    let paused = false;
    const { posts, gets } = stubDaemon({
      state: () => (paused ? goldenText("command-pause.json") : goldenText("state.json")),
      commands: { "POST /api/v1/pause": () => { paused = true; return json(goldenText("command-pause.json")); } }
    });
    const user = userEvent.setup();
    const controls = await renderShell();
    await waitFor(() => expect(gets("/api/v1/views/overview")).toBe(1));
    const overviewReads = gets("/api/v1/views/overview");
    await user.click(await within(controls).findByRole("button", { name: "Pause everything" }));
    expect(await within(controls).findByRole("button", { name: "Paused · Continue" })).toBeTruthy();
    expect(posts().length).toBe(1);
    await waitFor(() => expect(gets("/api/v1/state")).toBeGreaterThan(1));
    // The Overview reads paused state for its belt presentations, so a pause refetches it once and no more.
    expect(gets("/api/v1/views/overview")).toBe(overviewReads + 1);
    expect(within(controls).getByRole("button", { name: "Paused · Continue" })).toBeTruthy();
  });

  it("sends the AFK toggle and the empty Check now body the daemon expects", async () => {
    const { posts } = stubDaemon({
      commands: {
        "POST /api/v1/afk": () => json(goldenText("command-afk.json")),
        "POST /api/v1/check-now": () => json(goldenText("command-check-now.json"))
      }
    });
    const user = userEvent.setup();
    const controls = await renderShell();
    await user.click(await screen.findByRole("switch", { name: "Global AFK" }));
    await waitFor(() => expect(posts().length).toBe(1));
    expect(posts()[0]).toMatchObject({ path: "/api/v1/afk", body: { enabled: true } });
    await user.click(await within(controls).findByRole("button", { name: "Fetch" }));
    await waitFor(() => expect(posts().length).toBe(2));
    expect(posts()[1]).toMatchObject({ path: "/api/v1/check-now", body: {} });
  });

  it("shows a 422 without publishing a success, then offers no automatic retry", async () => {
    const { posts } = stubDaemon({
      commands: { "POST /api/v1/pause": () => json('{"message":"Command denied","problems":["daemon reason","daemon reason"]}', 422) }
    });
    const user = userEvent.setup();
    const controls = await renderShell();
    const pause = await within(controls).findByRole("button", { name: "Pause everything" });
    await user.click(pause);
    expect(await within(controls).findByText("Command denied")).toBeTruthy();
    expect(within(controls).getAllByText("daemon reason")).toHaveLength(2);
    expect(within(controls).getByRole("button", { name: "Pause everything" })).toBeTruthy();
    await new Promise(resolve => setTimeout(resolve, 20));
    expect(posts().length).toBe(1);
    expect(within(controls).queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("keeps a network failure visible with an explicit retry, and never repeats it on its own", async () => {
    let attempts = 0;
    const { posts } = stubDaemon({
      commands: {
        "POST /api/v1/check-now": () => {
          attempts += 1;
          if (attempts === 1) throw new TypeError("network down");
          return json(goldenText("command-check-now.json"));
        }
      }
    });
    const user = userEvent.setup();
    const controls = await renderShell();
    await user.click(await within(controls).findByRole("button", { name: "Fetch" }));
    expect(await within(controls).findByText("The daemon could not be reached.")).toBeTruthy();
    await new Promise(resolve => setTimeout(resolve, 20));
    expect(posts().length).toBe(1);
    await user.click(within(controls).getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(posts().length).toBe(2));
    expect(posts()[1]?.body).toEqual({});
  });

  it("disables the controls while offline and sends nothing when they are reached again", async () => {
    const { posts } = stubDaemon();
    const controls = await renderShell();
    await within(controls).findByRole("button", { name: "Pause everything" });
    await waitFor(() => expect(within(controls).getByRole("button", { name: "Pause everything" }).hasAttribute("disabled")).toBe(false));
    AppSocket.instances[0]?.close();
    await waitFor(() => expect(within(controls).getByRole("button", { name: "Pause everything" }).hasAttribute("disabled")).toBe(true));
    expect(within(controls).getAllByText("Unavailable while offline or reconnecting.").length).toBeGreaterThan(0);
    expect(posts()).toHaveLength(0);
  });

});
