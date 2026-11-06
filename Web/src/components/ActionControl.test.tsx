import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ActionControl, type ActionControlProps } from "./ActionControl";

const enabledPause = { id: "pause", label: "Pause", enabled: true, reason: null };

function renderControl(overrides: Partial<ActionControlProps> = {}) {
  const onActivate = vi.fn();
  const props: ActionControlProps = {
    id: "pause",
    fallbackLabel: "Pause",
    action: enabledPause,
    canRead: true,
    connection: "connected",
    pending: false,
    onActivate,
    ...overrides
  };
  const { container } = render(<ActionControl {...props} />);
  const button = container.querySelector("button") as HTMLButtonElement;
  return { onActivate, button };
}

function explanationOf(button: HTMLElement): string {
  const id = button.getAttribute("aria-describedby");
  return id ? document.getElementById(id)?.textContent ?? "" : "";
}

describe("ActionControl availability", () => {
  it("is available when the daemon enables the action on a compatible, online connection", async () => {
    const { onActivate, button } = renderControl();
    expect(button.hasAttribute("disabled")).toBe(false);
    expect(explanationOf(button)).toBe("");
    await userEvent.click(button);
    expect(onActivate).toHaveBeenCalledTimes(1);
  });

  it("exposes the daemon's disabled reason to the keyboard description", () => {
    const { button, onActivate } = renderControl({ action: { id: "pause", label: "Pause", enabled: false, reason: "Pause is not available." } });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("Pause is not available.");
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("cannot submit when the daemon does not offer the action", () => {
    const { button, onActivate } = renderControl({ action: undefined });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("The daemon does not offer this action.");
    expect(button.textContent).toBe("Pause");
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("refuses an unknown action id even when the daemon marks it enabled", () => {
    const { button, onActivate } = renderControl({ id: "custom", fallbackLabel: "Custom", action: { id: "custom", label: "Custom", enabled: true, reason: null } });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("Unsupported action.");
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("explains an offline connection", () => {
    const { button } = renderControl({ connection: "offline" });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("Unavailable while offline or reconnecting.");
  });

  it("explains a reconnecting connection alongside any daemon reason", () => {
    const { button } = renderControl({ connection: "connecting", action: { id: "pause", label: "Pause", enabled: false, reason: "Paused elsewhere." } });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("Paused elsewhere. Unavailable while offline or reconnecting.");
  });

  it("is unavailable until the API version is known and compatible", () => {
    const { button, onActivate } = renderControl({ canRead: false });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(explanationOf(button)).toBe("Waiting for the daemon API version.");
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("shows Working… and blocks activation while a command is pending", async () => {
    const { onActivate, button: working } = renderControl({ pending: true });
    expect(await screen.findByRole("button", { name: "Working…" })).toBe(working);
    expect(working.getAttribute("aria-busy")).toBe("true");
    expect(working.hasAttribute("disabled")).toBe(true);
    await userEvent.click(working);
    expect(onActivate).not.toHaveBeenCalled();
  });

  it("does not flash Working… for a fast command and clears its delayed label", async () => {
    vi.useFakeTimers();
    try {
      const props: ActionControlProps = { id: "pause", fallbackLabel: "Pause", action: enabledPause,
        canRead: true, connection: "connected", pending: false, onActivate: vi.fn() };
      const { rerender } = render(<ActionControl {...props} />);
      rerender(<ActionControl {...props} pending />);
      act(() => { vi.advanceTimersByTime(100); });
      expect(screen.getByRole("button", { name: "Pause" }).getAttribute("aria-busy")).toBe("true");
      rerender(<ActionControl {...props} />);
      act(() => { vi.advanceTimersByTime(200); });
      expect(screen.queryByRole("button", { name: "Working…" })).toBeNull();
      rerender(<ActionControl {...props} pending />);
      act(() => { vi.advanceTimersByTime(150); });
      expect(screen.getByRole("button", { name: "Working…" })).toBeTruthy();
      rerender(<ActionControl {...props} />);
      expect(screen.getByRole("button", { name: "Pause" }).hasAttribute("disabled")).toBe(false);
    } finally { vi.useRealTimers(); }
  });

  it("uses the primary style and the daemon's label when it is given", () => {
    const { button } = renderControl({ primary: true, action: { id: "pause", label: "Continue", enabled: true, reason: null } });
    expect(button.textContent).toBe("Continue");
    expect(button.className).toContain("fill");
  });

  it("is reached and activated from the keyboard", async () => {
    const user = userEvent.setup();
    const { onActivate, button } = renderControl();
    await user.tab();
    expect(document.activeElement).toBe(button);
    await user.keyboard("{Enter}");
    expect(onActivate).toHaveBeenCalledTimes(1);
  });
});
