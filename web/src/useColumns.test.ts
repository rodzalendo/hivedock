import { describe, expect, it } from "vitest";
import { effectiveColumns } from "./useColumns";

// The group grid buckets groups in JS and renders them with a grid-cols-N
// class. If the two ever disagree — which is what responsive classes caused,
// e.g. a 4-column preference rendering as lg:grid-cols-3 — the surplus bucket
// wraps onto a second grid row and its group drops below the tallest column
// instead of sitting beside its neighbours.
describe("effectiveColumns", () => {
  it("stacks to one column below the sm breakpoint", () => {
    for (const want of [1, 2, 3, 4]) {
      expect(effectiveColumns(want, 500)).toBe(1);
    }
  });

  it("never returns more columns than the user asked for", () => {
    for (const want of [1, 2, 3, 4]) {
      for (const width of [500, 700, 1100, 1400, 2560]) {
        expect(effectiveColumns(want, width)).toBeLessThanOrEqual(want);
      }
    }
  });

  it("caps a 4-column preference on a mid-width screen", () => {
    // The regression: 1100px renders 3 columns, so only 3 buckets may exist.
    expect(effectiveColumns(4, 1100)).toBe(3);
    expect(effectiveColumns(4, 800)).toBe(2);
    expect(effectiveColumns(4, 1400)).toBe(4);
  });

  it("caps a 3-column preference until xl", () => {
    expect(effectiveColumns(3, 1100)).toBe(2);
    expect(effectiveColumns(3, 1400)).toBe(3);
  });

  it("clamps out-of-range preferences", () => {
    expect(effectiveColumns(0, 1400)).toBe(1);
    expect(effectiveColumns(9, 1400)).toBe(4);
  });
});
