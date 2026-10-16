export const STANDARD_COLORS = [
  "Black",
  "Blue",
  "Gold",
  "Gray",
  "Green",
  "Orange",
  "Pink",
  "Purple",
  "Red",
  "Silver",
  "White",
  "Yellow",
] as const;

export type StandardColor = (typeof STANDARD_COLORS)[number];

// Catalog aliases map to their visible color family, independently of model.
const aliases: Record<string, StandardColor> = {
  "alpine green": "Green",
  "black titanium": "Black",
  "blue titanium": "Blue",
  "cloud white": "White",
  "cosmic orange": "Orange",
  "deep blue": "Blue",
  "deep purple": "Purple",
  "desert titanium": "Gold",
  graphite: "Gray",
  grey: "Gray",
  lavender: "Purple",
  "light gold": "Gold",
  midnight: "Black",
  "midnight green": "Green",
  "mist blue": "Blue",
  "natural titanium": "Gray",
  "pacific blue": "Blue",
  sage: "Green",
  "sierra blue": "Blue",
  "sky blue": "Blue",
  "soft pink": "Pink",
  "space black": "Black",
  "space gray": "Gray",
  "space grey": "Gray",
  starlight: "White",
  teal: "Green",
  ultramarine: "Blue",
  "white titanium": "White",
  "(product)red": "Red",
};

export function normalizeColorFamily(name: string): StandardColor | undefined {
  const normalized = name.trim().toLowerCase().replace(/\s+/g, " ");
  return (
    STANDARD_COLORS.find((color) => color.toLowerCase() === normalized) ??
    aliases[normalized]
  );
}

export function toColorFamilies(colors: { name: string }[]): StandardColor[] {
  const families = new Set(
    colors.map(({ name }) => normalizeColorFamily(name)),
  );
  return STANDARD_COLORS.filter((color) => families.has(color));
}
