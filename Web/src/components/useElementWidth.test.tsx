import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useElementWidth } from "./useElementWidth";

let laidOutWidth = 0;
let fireResize: () => void = () => undefined;

class FakeResizeObserver {
  constructor(callback: () => void) { fireResize = callback; }
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

function Probe() {
  const [ref, width] = useElementWidth<HTMLDivElement>();
  return <div ref={ref}><output>{width}</output></div>;
}

describe("useElementWidth", () => {
  beforeEach(() => {
    laidOutWidth = 640.6;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(() => ({ width: laidOutWidth }) as DOMRect);
    vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("reports the laid-out width rounded down on mount, so content sized to it never overflows", () => {
    render(<Probe />);
    expect(document.querySelector("output")?.textContent).toBe("640");
  });

  it("measures again on every resize", () => {
    render(<Probe />);
    laidOutWidth = 800;
    act(() => fireResize());
    expect(document.querySelector("output")?.textContent).toBe("800");
  });
});
