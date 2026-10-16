import {
  normalizeColorFamily,
  STANDARD_COLORS,
  toColorFamilies,
} from "../search/helpers/color-families"

describe("standard color families", () => {
  it.each(STANDARD_COLORS)("preserves %s", (color) => {
    expect(normalizeColorFamily(color)).toBe(color);
  });

  it.each([
    ["Midnight", "Black"],
    ["Midnight Green", "Green"],
    ["Starlight", "White"],
    ["Graphite", "Gray"],
    ["Natural Titanium", "Gray"],
    ["Desert Titanium", "Gold"],
    ["Teal", "Green"],
    ["Ultramarine", "Blue"],
    ["Lavender", "Purple"],
    ["Cosmic Orange", "Orange"],
    ["  BLACK   TITANIUM ", "Black"],
  ])("maps %s to %s", (name, color) => {
    expect(normalizeColorFamily(name)).toBe(color);
  });

  it("deduplicates aliases and leaves unknown colors unclassified", () => {
    expect(
      toColorFamilies([
        { name: "Blue" },
        { name: "Sierra Blue" },
        { name: "Black Titanium" },
        { name: "Unknown finish" },
      ]),
    ).toEqual(["Black", "Blue"]);
    expect(normalizeColorFamily("Unknown finish")).toBeUndefined();
  });
});
