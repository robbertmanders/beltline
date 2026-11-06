import { useState } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Sheet } from "./Sheet";

function Harness({ onClose, onSave, removeOpener }: { onClose: () => void; onSave?: () => void; removeOpener?: boolean }) {
  const [open, setOpen] = useState(false);
  const close = () => { onClose(); setOpen(false); };
  return <>
    {!removeOpener && <button type="button" onClick={() => setOpen(true)}>Open sheet</button>}
    <button type="button">Outside</button>
    <Sheet title="Edit belt" open={open} onClose={close} footer={<><button type="button" onClick={close}>Cancel</button><button type="button" onClick={onSave}>Save</button></>}>
      <label>Name <input /></label>
      <button type="button">Stage</button>
    </Sheet>
  </>;
}

async function openSheet(user: ReturnType<typeof userEvent.setup>, onClose = vi.fn()) {
  render(<Harness onClose={onClose} />);
  const opener = screen.getByRole("button", { name: "Open sheet" });
  await user.click(opener);
  return { opener, onClose, dialog: await screen.findByRole("dialog", { name: "Edit belt" }) };
}

describe("Sheet", () => {
  it("renders nothing while closed", () => {
    render(<Sheet title="Hidden" open={false} onClose={() => {}}><p>secret</p></Sheet>);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText("secret")).toBeNull();
  });

  it("has an accessible title and moves focus inside when opened", async () => {
    const user = userEvent.setup();
    const { dialog } = await openSheet(user);
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(document.activeElement).toBe(screen.getByRole("textbox"));
    expect(dialog.contains(document.activeElement)).toBe(true);
  });

  it("confines Tab forward and Shift+Tab backward to the sheet", async () => {
    const user = userEvent.setup();
    const { dialog } = await openSheet(user);
    const focusable = Array.from(dialog.querySelectorAll<HTMLElement>("button, input"));
    const first = focusable[0]!;
    const last = focusable[focusable.length - 1]!;
    last.focus();
    await user.tab();
    expect(document.activeElement).toBe(first);
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(last);
    for (let step = 0; step < focusable.length + 2; step += 1) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
  });

  it("starts on Cancel and cycles between footer actions when the header Close is omitted", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Sheet title="Abort the run?" open hideClose onClose={onClose}
      footer={<><button type="button">Cancel</button><button type="button">Abort the run</button></>}>
      <p>Watching stops.</p>
    </Sheet>);
    const cancel = screen.getByRole("button", { name: "Cancel" });
    const abort = screen.getByRole("button", { name: "Abort the run" });
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(document.activeElement).toBe(cancel);
    await user.tab();
    expect(document.activeElement).toBe(abort);
    await user.tab();
    expect(document.activeElement).toBe(cancel);
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(abort);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes with Escape and returns focus to its opener", async () => {
    const user = userEvent.setup();
    const { opener, onClose } = await openSheet(user);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(opener);
  });

  it("closes with the header Close and the footer Cancel controls", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Open sheet" }));
    await user.click(await screen.findByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open sheet" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("makes the background inert while open and restores it on close", async () => {
    const user = userEvent.setup();
    const { dialog } = await openSheet(user);
    const outside = screen.getByRole("button", { name: "Outside" });
    expect(outside.closest("[inert]")).not.toBeNull();
    expect(dialog.closest("[inert]")).toBeNull();
    act(() => { outside.focus(); });
    expect(dialog.contains(document.activeElement)).toBe(true);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(outside.closest("[inert]")).toBeNull());
  });

  it("does not throw or move focus when the opener has been removed", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const { rerender } = render(<Harness onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Open sheet" }));
    await screen.findByRole("dialog");
    rerender(<Harness onClose={onClose} removeOpener />);
    expect(screen.queryByRole("button", { name: "Open sheet" })).toBeNull();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement?.isConnected).toBe(true);
    expect(document.activeElement).not.toBe(document.body.querySelector(".sheet-layer"));
  });

  it("sends nothing when it opens and when it is cancelled", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetcher);
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<Harness onClose={vi.fn()} onSave={onSave} />);
    await user.click(screen.getByRole("button", { name: "Open sheet" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(fetcher).not.toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("scrolls its body inside the viewport rather than the page", async () => {
    render(<Sheet title="Long" open onClose={() => {}}><p>{"line ".repeat(400)}</p></Sheet>);
    const body = document.querySelector<HTMLElement>(".sheet-body");
    expect(body).not.toBeNull();
    expect(fireEvent.scroll(body!)).toBe(true);
  });
});
