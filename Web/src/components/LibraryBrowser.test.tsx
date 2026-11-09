import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LibraryBrowser, type LibraryBrowserProps } from "./LibraryBrowser";
import type { LibraryBrowser as Browser } from "../api/wire.gen";

const connection = vi.hoisted(() => ({ canRead: true, connection: "connected" }));
vi.mock("../app/runtime", () => ({ useRuntime: () => connection }));

const browser: Browser = {
  items: [{ id: "triage", name: "Triage", version: 2, summary: "Choose what needs doing.", detail: "Codex · 3 belts",
    status: "In use", statusTone: "good", icon: "funnel", tint: "blue", stages: [] }],
  filters: [{ id: "all", label: "All live", count: 12 }, { id: "unused", label: "Unused", count: 4 }],
  totalCount: 15, matchCount: 1,
  actions: [{ id: "new", label: "New agent", enabled: true }]
};

function props(overrides: Partial<LibraryBrowserProps> = {}): LibraryBrowserProps {
  return { title: "Agents", lead: "Your reusable agents.", browser, search: "", filter: "all", onSearch: vi.fn(), onFilter: vi.fn(),
    onOpen: vi.fn(), onNew: vi.fn(), onRetry: vi.fn(), ...overrides };
}

beforeEach(() => { connection.canRead = true; connection.connection = "connected"; });

describe("Library browser", () => {
  it("renders supplied cards and counts, and opens a card with the keyboard", async () => {
    const inputs = props({ search: "a query the card itself does not match" });
    render(<LibraryBrowser {...inputs} />);
    expect(screen.getByRole("button", { name: "All live 12" })).toBeTruthy();
    expect(screen.getByText("1 shown · 15 in the library")).toBeTruthy();
    expect(screen.getByText("In use")).toBeTruthy();
    const card = screen.getByRole("button", { name: "Open Triage" });
    card.focus();
    await userEvent.keyboard("{Enter}");
    expect(inputs.onOpen).toHaveBeenCalledWith("triage");
  });

  it("forwards search, filter, clear and New without deriving results", async () => {
    const inputs = props({ search: "tri" });
    render(<LibraryBrowser {...inputs} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Search agents" }), { target: { value: "build" } });
    expect(inputs.onSearch).toHaveBeenCalledWith("build");
    await userEvent.click(screen.getByRole("button", { name: "Clear Agents search" }));
    expect(inputs.onSearch).toHaveBeenCalledWith("");
    await userEvent.click(screen.getByRole("button", { name: "Unused 4" }));
    expect(inputs.onFilter).toHaveBeenCalledWith("unused");
    expect(screen.queryByRole("button", { name: "Refresh" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "New agent" }));
    expect(inputs.onNew).toHaveBeenCalledOnce();
  });

  it("requires the daemon's New action and a connected runtime", async () => {
    const inputs = props({ browser: { ...browser, actions: [] } });
    const view = render(<LibraryBrowser {...inputs} />);
    expect((screen.getByRole("button", { name: "New agent" }) as HTMLButtonElement).disabled).toBe(true);
    view.rerender(<LibraryBrowser {...inputs} browser={{ ...browser, actions: [{ id: "new", label: "New agent", enabled: false, reason: "Library is locked." }] }} />);
    expect(screen.getByText("Library is locked.")).toBeTruthy();
    connection.connection = "offline";
    view.rerender(<LibraryBrowser {...inputs} browser={browser} />);
    await userEvent.click(screen.getByRole("button", { name: "New agent" }));
    expect(inputs.onNew).not.toHaveBeenCalled();
  });

  it("distinguishes an empty library, no matches, and a stale refresh", async () => {
    const inputs = props({ browser: { ...browser, items: [], totalCount: 0, matchCount: 0 } });
    const view = render(<LibraryBrowser {...inputs} />);
    expect(screen.getByRole("heading", { name: "No agents yet" })).toBeTruthy();
    view.rerender(<LibraryBrowser {...inputs} browser={{ ...browser, items: [], matchCount: 0 }} />);
    expect(screen.getByRole("heading", { name: "No agents match" })).toBeTruthy();
    view.rerender(<LibraryBrowser {...inputs} browser={browser} error={new Error("Daemon is unavailable.")} />);
    expect(screen.getByRole("button", { name: "Open Triage" })).toBeTruthy();
    expect(screen.getByText("Showing the last loaded library; refresh failed.")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(inputs.onRetry).toHaveBeenCalledOnce();
  });

  it("restores scroll after returning from detail and keeps drafts accessible", async () => {
    const onDraft = vi.fn();
    const inputs = props({ scrollKey: "scroll-test", drafts: [{ id: "new", name: "My draft", onOpen: onDraft }] });
    const first = render(<LibraryBrowser {...inputs} />);
    const main = screen.getByRole("main");
    main.scrollTop = 180;
    fireEvent.scroll(main);
    first.unmount();
    render(<LibraryBrowser {...inputs} />);
    expect(screen.getByRole("main").scrollTop).toBe(180);
    await userEvent.click(screen.getByRole("button", { name: "My draft Draft" }));
    expect(onDraft).toHaveBeenCalledOnce();
  });

  it("shows a refused command separately from refresh errors and retries explicitly", async () => {
    const onRetryCommand = vi.fn();
    render(<LibraryBrowser {...props({ commandError: new Error("The library is locked."), onRetryCommand })} />);
    expect(screen.getByText("The library is locked.")).toBeTruthy();
    expect(screen.queryByText("Showing the last loaded library; refresh failed.")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Retry command" }));
    expect(onRetryCommand).toHaveBeenCalledOnce();
  });
});
