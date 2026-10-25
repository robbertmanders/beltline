import { describe, expect, it } from "vitest";
import { shippedCurve } from "./shippedChart";

describe("shippedCurve", () => {
  it("runs from the first week to the last, closing its area along the base", () => {
    const weeks = Array.from({ length: 12 }, () => 0);
    weeks[11] = 7;
    const curve = shippedCurve(weeks);
    expect(curve.line.startsWith("M10 120 C")).toBe(true);
    expect(curve.area.endsWith("L290 120 L10 120 Z")).toBe(true);
    expect(curve.dotX).toBe(290);
    // The last week is the peak, so its point sits at the top of the curve's range: 120 - 100 * 7 / (7 * 8 / 7).
    expect(curve.dotY).toBe(32.5);
  });

  it("keeps an empty quarter on the base", () => {
    const curve = shippedCurve(Array.from({ length: 12 }, () => 0));
    expect(curve.dotY).toBe(120);
    expect(curve.line).not.toContain("NaN");
  });
});
