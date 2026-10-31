import { screen, waitFor, within, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppProviders } from "../../app/App";
import { fixtureRoute } from "../../test/daemon";
import { json } from "../../test/response";
import { LibraryPage } from "./LibraryPage";
import { backupRepo, mockLibrary, sharedRepo } from "./libraryTestKit";

const repos = (...list: unknown[]) => JSON.stringify({ repos: list });

/** The golden's shared belt and agent, as a card opens them. */
const beltPath = "/library?tab=belt&repository=example%2Fshared&path=&id=team-nightly";
const agentPath = "/library?tab=agent&repository=example%2Fshared&path=&id=team-build";

function renderLibrary(path = "/library") {
  render(<AppProviders><MemoryRouter initialEntries={[path]}><LibraryPage /></MemoryRouter></AppProviders>);
}

/** A control waits for the daemon connection before it can be used, as every command control does. */
async function usable(button: HTMLElement): Promise<HTMLElement> {
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
  return button;
}

/** The query of the newest request to path, as search parameters. */
function lastQuery(requests: readonly { path: string }[], path: string): URLSearchParams {
  const found = requests.filter(request => request.path.startsWith(`${path}?`));
  return new URLSearchParams(found[found.length - 1]!.path.split("?")[1]);
}

describe("Library page", () => {
  it("shows each repository with its role and the daemon's actions", async () => {
    mockLibrary({ "/api/v1/library/repos": repos(backupRepo, sharedRepo) });
    renderLibrary();
    const backup = await screen.findByRole("region", { name: "Library acme/beltline-backup" });
    expect(within(backup).getByText("Changed since backup")).toBeTruthy();
    expect((await usable(within(backup).getByRole("button", { name: "Back up" })) as HTMLButtonElement).disabled).toBe(false);
    const shared = screen.getByRole("region", { name: "Library acme/team-library" });
    expect(within(shared).getByText("Share opens a pull request")).toBeTruthy();
    expect(within(shared).queryByRole("button", { name: "Back up" })).toBeNull();
  });

  it("lists the Belts tab's cards with their state and repository, and switches to Agents", async () => {
    const { requests } = mockLibrary({ "/api/v1/library/repos": repos(sharedRepo) });
    const user = userEvent.setup();
    renderLibrary();
    const belt = await screen.findByRole("button", { name: "Open Issue flow" });
    expect(within(belt).getByText("Update")).toBeTruthy();
    expect(within(belt).getByText("acme/team-library")).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Belts/ }).getAttribute("aria-selected")).toBe("true");
    expect(lastQuery(requests, "/api/v1/library/browser").get("kind")).toBe("belt");
    await user.click(screen.getByRole("tab", { name: /Agents/ }));
    const triage = await screen.findByRole("button", { name: "Open Triage" });
    expect(within(triage).getByText("Conflict")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Conflicts 1" })).toBeTruthy();
    expect(lastQuery(requests, "/api/v1/library/browser").get("kind")).toBe("agent");
  });

  it("sends the search and the state filter to the daemon", async () => {
    const { requests } = mockLibrary({ "/api/v1/library/repos": repos(sharedRepo) });
    const user = userEvent.setup();
    renderLibrary("/library?tab=agent");
    await user.click(await screen.findByRole("button", { name: "Installed 1" }));
    await waitFor(() => expect(lastQuery(requests, "/api/v1/library/browser").get("filter")).toBe("installed"));
    await user.type(screen.getByRole("textbox", { name: "Search agents" }), "rev");
    await waitFor(() => expect(lastQuery(requests, "/api/v1/library/browser").get("search")).toBe("rev"));
  });

  it("opens a card's belt in the belt editor, read-only, and a stage's form with its brief", async () => {
    const { requests } = mockLibrary({ "/api/v1/library/repos": repos(sharedRepo), "/api/v1/library/belt": fixtureRoute("library-belt.json") });
    const user = userEvent.setup();
    renderLibrary(beltPath);
    expect(await screen.findByRole("heading", { name: "Nightly (team)" })).toBeTruthy();
    expect(screen.getByText("Belt · v1 · example/shared")).toBeTruthy();
    const line = screen.getByRole("region", { name: "Stages" });
    expect(within(line).queryByRole("button", { name: "Add a stage here" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add trigger" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Add a repository to trigger 1" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Pause belt|Archive|Share…|Save/ })).toBeNull();

    await user.click(within(line).getByRole("button", { name: "Sweep" }));
    await waitFor(() => expect(lastQuery(requests, "/api/v1/library/belt").get("stage")).toBe("build"));
    const settings = await screen.findByRole("region", { name: "Stage settings" });
    expect(within(settings).getByRole("button", { name: "Open agent" })).toBeTruthy();
    expect(within(settings).queryByRole("button", { name: "Remove stage" })).toBeNull();
    expect(within(settings).getByRole("textbox", { name: "Name" }).matches(":disabled")).toBe(true);
    const brief = screen.getByRole("textbox", { name: "Brief" }) as HTMLTextAreaElement;
    expect(brief.readOnly).toBe(true);
    expect(brief.value).toContain("Commit your work on the worktree branch {branch}.");
    expect(screen.queryByRole("button", { name: "+ Add outcome" })).toBeNull();
  });

  it("installs the open belt with its repository, kind and id", async () => {
    const { posts } = mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/belt": fixtureRoute("library-belt.json"),
      "/api/v1/library/install": () => json({ written: ["belts/team-nightly/v1.json"] })
    });
    const user = userEvent.setup();
    renderLibrary(beltPath);
    await user.click(await usable(await screen.findByRole("button", { name: "Install" })));
    await waitFor(() => expect(posts("/library/install")).toHaveLength(1));
    expect(posts("/library/install")[0]!.body).toEqual({ repository: "example/shared", path: "", kind: "belt", id: "team-nightly" });
    expect(await screen.findByText(/Installed v1\./)).toBeTruthy();
  });

  it("opens a shared agent in the agent editor, read-only, and its belt from Used by", async () => {
    const { requests } = mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/agent": fixtureRoute("library-agent.json"),
      "/api/v1/library/belt": fixtureRoute("library-belt.json")
    });
    const user = userEvent.setup();
    renderLibrary(agentPath);
    expect(await screen.findByRole("heading", { name: "Build" })).toBeTruthy();
    expect(screen.getByText("Agent · v1 · example/shared")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Name" }).matches(":disabled")).toBe(true);
    expect((screen.getByRole("textbox", { name: "Brief" }) as HTMLTextAreaElement).readOnly).toBe(true);
    expect(screen.queryByRole("button", { name: /Share…|Archive|\+ Add outcome|\+ Add a model/ })).toBeNull();
    expect(screen.getByRole("heading", { name: "Used by 1 belt in this repository" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Open belt" }));
    expect(await screen.findByRole("heading", { name: "Nightly (team)" })).toBeTruthy();
    expect(lastQuery(requests, "/api/v1/library/belt").get("id")).toBe("team-nightly");
  });

  it("compares the open item's files beside your own copy", async () => {
    const files = { files: [{ path: "belts/team-nightly/v1.json", repo: '{"version":1}', local: null }] };
    const { requests } = mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/belt": fixtureRoute("library-belt.json"),
      "/api/v1/library/item": () => json(files)
    });
    const user = userEvent.setup();
    renderLibrary(beltPath);
    await user.click(await screen.findByRole("button", { name: "Compare" }));
    const sheet = await screen.findByRole("dialog", { name: "Compare Nightly (team)" });
    expect(await within(sheet).findByText("belts/team-nightly/v1.json")).toBeTruthy();
    expect(within(sheet).getByText("Not in your library yet.")).toBeTruthy();
    expect(lastQuery(requests, "/api/v1/library/item").get("id")).toBe("team-nightly");
  });

  it("backs up and says whether anything changed", async () => {
    const { posts } = mockLibrary({
      "/api/v1/library/repos": repos(backupRepo),
      "/api/v1/library/backup": () => json({ commit: "7d1e0aa", changed: true })
    });
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await usable(await screen.findByRole("button", { name: "Back up" })));
    expect(await screen.findByText("Backed up as 7d1e0aa.")).toBeTruthy();
    expect(posts("/library/backup")[0]!.body).toEqual({ repository: "acme/beltline-backup", path: "" });
  });

  it("asks before a restore and sends it only when confirmed", async () => {
    const { posts } = mockLibrary({
      "/api/v1/library/repos": repos(backupRepo),
      "/api/v1/library/restore": () => json({ savedTo: "library-backups/2026-10-15T10-00-00" })
    });
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await usable(await screen.findByRole("button", { name: "Restore…" })));
    const confirm = await screen.findByRole("alertdialog", { name: "Confirm restore" });
    expect(posts("/library/restore")).toHaveLength(0);
    await user.click(await usable(within(confirm).getByRole("button", { name: "Restore" })));
    expect(await screen.findByText("The previous library was saved to library-backups/2026-10-15T10-00-00.")).toBeTruthy();
    expect(posts("/library/restore")[0]!.body).toEqual({ repository: "acme/beltline-backup", path: "" });
  });

  it("shows a daemon refusal of a command as its problems", async () => {
    mockLibrary({
      "/api/v1/library/repos": repos(backupRepo),
      "/api/v1/library/backup": () => json({ message: "Backup failed", problems: ["git push was rejected"] }, 422)
    });
    const user = userEvent.setup();
    renderLibrary();
    await user.click(await usable(await screen.findByRole("button", { name: "Back up" })));
    expect(await screen.findByText("Backup failed")).toBeTruthy();
    expect(screen.getByText("git push was rejected")).toBeTruthy();
  });

  it("shows a refresh that timed out in the repository's error area, and Refresh runs again", async () => {
    let refreshes = 0;
    const { posts } = mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/refresh": () => {
        refreshes += 1;
        return refreshes === 1 ? json({ message: "git fetch failed: command timed out", problems: [] }, 422) : json(sharedRepo);
      }
    });
    const user = userEvent.setup();
    renderLibrary();
    const shared = await screen.findByRole("region", { name: "Library acme/team-library" });
    await user.click(await usable(within(shared).getByRole("button", { name: "Refresh" })));
    const alert = await within(shared).findByRole("alert");
    expect(alert.textContent).toContain("git fetch failed: command timed out");
    await user.click(await usable(within(shared).getByRole("button", { name: "Refresh" })));
    await waitFor(() => expect(posts("/library/refresh")).toHaveLength(2));
    await waitFor(() => expect(within(shared).queryByRole("alert")).toBeNull());
    expect((within(shared).getByRole("button", { name: "Refresh" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("points to Settings when no library repository is set up", async () => {
    mockLibrary({ "/api/v1/library/repos": repos() });
    renderLibrary();
    expect(await screen.findByRole("link", { name: "Add one in Settings → Library repositories" })).toBeTruthy();
  });
});
