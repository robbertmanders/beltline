import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import "../theme/tokens.css";
import "../theme/fields.css";
import Gallery from "./Gallery";

describe("development theme gallery", () => {
  it("shows all tokens, fonts and controls without a daemon", () => {
    const { container } = render(<Gallery />);
    expect(screen.getByRole("heading", { name: "Theme gallery" })).toBeTruthy();
    expect(container.querySelectorAll(".gallery-token")).toHaveLength(125);
    expect(container.querySelectorAll(".gallery-font-sample")).toHaveLength(3);
    expect(container.querySelectorAll(".gallery-field-card")).toHaveLength(4);
  });

  it("keeps read-only and disabled samples immutable and allows keyboard segment selection", async () => {
    const user = userEvent.setup();
    render(<Gallery />);
    const readOnly = screen.getByRole("textbox", { name: "Read-only paper field" }) as HTMLInputElement;
    const disabled = screen.getByRole("textbox", { name: "Disabled paper field" }) as HTMLInputElement;
    expect(readOnly.readOnly).toBe(true);
    expect(disabled.disabled).toBe(true);
    await user.click(readOnly);
    await user.keyboard(" changed");
    expect(readOnly.value).toBe("Read-only sample");
    const readOnlySelect = screen.getByRole("combobox", { name: "Read-only select sample" }) as HTMLSelectElement;
    expect(readOnlySelect.disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Disabled · 40%" }) as HTMLButtonElement).disabled).toBe(true);

    const group = screen.getByRole("group", { name: "Segmented selection" });
    const web = within(group).getByRole("button", { name: "Web" });
    await user.tab();
    await user.click(web);
    expect(web.getAttribute("aria-pressed")).toBe("true");
    expect(within(group).getByRole("button", { name: "Native" }).getAttribute("aria-pressed")).toBe("false");
  });
});
