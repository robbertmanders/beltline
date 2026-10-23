import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ModelChoice, ModelRunnerResource } from "../api/wire.gen";
import { golden } from "../test/golden";
import { ModelPicker, type ModelPickerProps } from "./ModelPicker";

const models = golden<ModelRunnerResource>("models.json");
const choices: ModelChoice[] = models.choices;

function renderPicker(overrides: Partial<ModelPickerProps> = {}) {
  const onChange = vi.fn();
  const props: ModelPickerProps = { id: "stage-model", label: "Model", runnerLabel: "Pi", choices, allowedModels: models.allowedModels, value: "", onChange, ...overrides };
  const { container, rerender } = render(<ModelPicker {...props} />);
  const select = screen.queryByRole("combobox") as HTMLSelectElement | null;
  return { onChange, select, container, rerender: (next: Partial<ModelPickerProps>) => rerender(<ModelPicker {...props} {...next} />) };
}

describe("ModelPicker choices", () => {
  it("offers exactly the daemon's choices, labelled with their names and reported context sizes", () => {
    const { select } = renderPicker({ value: "opus-5.5-1m" });
    const options = Array.from(select!.options).map(option => ({ value: option.value, label: option.textContent, disabled: option.disabled }));
    expect(options.filter(option => !option.disabled)).toEqual([
      { value: "fixture-model", label: "Fixture model (demo)", disabled: false },
      { value: "openrouter/stealth/space-bunny-alpha", label: "Fixture Pi model (demo)", disabled: false },
      { value: "opus-5.5-1m", label: "Opus 5.5 - 1M", disabled: false },
      { value: "opus-5.5-250k", label: "Opus 5.5 - 250k", disabled: false }
    ]);
  });

  it("returns the unchanged opaque id when a choice is picked", async () => {
    const user = userEvent.setup();
    const { onChange, select } = renderPicker({ value: "fixture-model" });
    await user.selectOptions(select!, "opus-5.5-250k");
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith("opus-5.5-250k");
  });

  it("never calls back for a value outside the choices, including the runner default placeholder", () => {
    const { onChange, select } = renderPicker({ value: "fixture-model" });
    fireEvent.change(select!, { target: { value: "not-offered" } });
    fireEvent.change(select!, { target: { value: "" } });
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows the runner default without a selectable default option", () => {
    const { select } = renderPicker({ value: "" });
    expect(select!.value).toBe("");
    const placeholder = within(select!).getByRole("option", { name: "Runner default", hidden: true }) as HTMLOptionElement;
    expect(placeholder.disabled).toBe(true);
    expect(Array.from(select!.options).filter(option => option.value === "" && !option.disabled)).toEqual([]);
  });

  it("keeps a selection the daemon no longer allows visible as not allowed", () => {
    const { select } = renderPicker({ value: "secret-model", choices: choices.filter(choice => choice.id !== "secret-model") });
    expect(select!.value).toBe("secret-model");
    expect(screen.getByText("not allowed")).toBeTruthy();
    expect(within(select!).getByRole("option", { name: "secret-model (not allowed)", hidden: true }).hasAttribute("disabled")).toBe(true);
  });

  it("keeps an allowed selection that is missing from the catalog as unavailable in catalog", () => {
    const { select } = renderPicker({ value: "opus-5.5-gone", allowedModels: ["opus-5.5-gone"] });
    expect(select!.value).toBe("opus-5.5-gone");
    expect(screen.getByText("unavailable in catalog")).toBeTruthy();
  });

  it("does not replace the input when the runner or choices change", () => {
    const onChange = vi.fn();
    const { rerender, select } = renderPicker({ value: "opus-5.5-1m", onChange });
    rerender({ runnerLabel: "Claude Code", choices: [{ id: "other", name: "Other" }] });
    expect(onChange).not.toHaveBeenCalled();
    expect(select!.value).toBe("opus-5.5-1m");
    // The model is still allowed, so its absence from the new choices means the catalog no longer has it.
    expect(screen.getByText("unavailable in catalog")).toBeTruthy();
  });
});

describe("ModelPicker states", () => {
  it("shows the empty state for a runner with no models, distinct from loading", () => {
    const { select, container } = renderPicker({ choices: [], allowedModels: [], value: "" });
    expect(container.querySelector("[data-model-state]")?.getAttribute("data-model-state")).toBe("empty");
    expect(select!.disabled).toBe(true);
    expect(screen.getByText("No models reported for Pi.")).toBeTruthy();
  });

  it("shows loading as a status and failure as an alert with its message", () => {
    const loading = renderPicker({ state: "loading" });
    expect(screen.getByRole("status").textContent).toBe("Loading models…");
    expect(loading.select!.disabled).toBe(true);
    loading.container.remove();
    const failed = renderPicker({ state: "failed", errorMessage: "pi exited with status 1" });
    expect(screen.getByRole("alert").textContent).toBe("pi exited with status 1");
    expect(failed.select!.disabled).toBe(true);
  });

  it("freezes the choices of a run and keeps the selection visible", () => {
    const { select, container } = renderPicker({ state: "frozen", value: "fixture-model" });
    expect(container.querySelector("[data-model-state]")?.getAttribute("data-model-state")).toBe("frozen");
    expect(select!.disabled).toBe(true);
    expect(select!.value).toBe("fixture-model");
    expect(screen.getByText("Choices are frozen for this run.")).toBeTruthy();
  });

  it("shows a read-only publisher's model id exactly as written, with no select", () => {
    const publisherId = "Vendor/Model_ID.v2:latest";
    const { select, container } = renderPicker({ state: "readonly", value: publisherId });
    expect(select).toBeNull();
    expect(container.querySelector("[data-model-state]")?.getAttribute("data-model-state")).toBe("readonly");
    expect(container.querySelector("code")?.textContent).toBe(publisherId);
  });

  it("shows a read-only runner default when the id is empty", () => {
    const { container } = renderPicker({ state: "readonly", value: "" });
    expect(container.querySelector("code")?.textContent).toBe("Runner default");
  });
});
