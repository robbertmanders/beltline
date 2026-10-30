import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppProviders } from "../../app/App";
import { json } from "../../test/response";
import { ShareSheet, type ShareSource } from "./ShareSheet";
import { backupRepo, mockLibrary, sharedRepo } from "./libraryTestKit";

const source: ShareSource = { kind: "belt", id: "issue-flow", version: 2, name: "Issue flow" };
const repos = (...list: unknown[]) => JSON.stringify({ repos: list });
const pushRepo = { ...sharedRepo, publish: "push" };

function renderShare(onClose = () => undefined, item: ShareSource = source) {
  render(<AppProviders><MemoryRouter><ShareSheet source={item} onClose={onClose} /></MemoryRouter></AppProviders>);
}

/** The Share button waits for the daemon connection, as every command control does. */
async function shareButton(): Promise<HTMLElement> {
  const button = await screen.findByRole("button", { name: "Share" });
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
  return button;
}

describe("Share sheet", () => {
  it("opens a pull request on the shared repository by default and links it", async () => {
    const { posts } = mockLibrary({
      "/api/v1/library/repos": repos(backupRepo, sharedRepo),
      "/api/v1/library/share": () => json({ pullRequest: "https://github.com/acme/team-library/pull/12", commit: "b3c1e0d", changed: true })
    });
    const user = userEvent.setup();
    renderShare();
    const sheet = await screen.findByRole("dialog", { name: "Share Issue flow" });
    expect(within(sheet).getByText("issue-flow · v2")).toBeTruthy();
    expect(await within(sheet).findByText("Opens a pull request on acme/team-library for review.")).toBeTruthy();
    await user.click(await shareButton());
    expect(posts("/library/share")[0]!.body).toEqual({ repository: "acme/team-library", path: "", kind: "belt", id: "issue-flow" });
    expect(await within(sheet).findByRole("link", { name: "Open the pull request" })).toBeTruthy();
    expect((await screen.findByRole("button", { name: "Done" })).textContent).toBe("Done");
  });

  it("never offers a repository that takes no shares, such as the examples repository", async () => {
    const examples = { ...sharedRepo, repository: "robbertmanders/beltline-examples", takesShares: false };
    mockLibrary({ "/api/v1/library/repos": repos(examples) });
    renderShare();
    expect(await screen.findByText(/No shared repository is set up/)).toBeTruthy();
    expect(screen.queryByText(/robbertmanders\/beltline-examples/)).toBeNull();
  });

  it("says what a push did when the repository publishes by push", async () => {
    mockLibrary({
      "/api/v1/library/repos": repos(pushRepo),
      "/api/v1/library/share": () => json({ pullRequest: null, commit: "b3c1e0d", changed: true })
    });
    const user = userEvent.setup();
    renderShare();
    expect(await screen.findByText("Pushes straight to the default branch of acme/team-library.")).toBeTruthy();
    await user.click(await shareButton());
    expect(await screen.findByText("Pushed as b3c1e0d.")).toBeTruthy();
  });

  it("says when the repository already holds these contents", async () => {
    mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/share": () => json({ pullRequest: null, commit: null, changed: false })
    });
    const user = userEvent.setup();
    renderShare();
    await user.click(await shareButton());
    expect(await screen.findByText("Already in acme/team-library with these contents.")).toBeTruthy();
  });

  it("shows the daemon's refusal beside the sheet", async () => {
    mockLibrary({
      "/api/v1/library/repos": repos(sharedRepo),
      "/api/v1/library/share": () => json({ message: "Belt issue-flow v2 already exists with other content", problems: ["Save a new version first"] }, 422)
    });
    const user = userEvent.setup();
    renderShare();
    await user.click(await shareButton());
    expect(await screen.findByText("Belt issue-flow v2 already exists with other content")).toBeTruthy();
    expect(screen.getByText("Save a new version first")).toBeTruthy();
  });

  it("explains how to add a shared repository when none is set up", async () => {
    mockLibrary({ "/api/v1/library/repos": repos(backupRepo) });
    renderShare();
    expect(await screen.findByText("No shared repository is set up. Add one in Settings → Library repositories first.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Share" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
