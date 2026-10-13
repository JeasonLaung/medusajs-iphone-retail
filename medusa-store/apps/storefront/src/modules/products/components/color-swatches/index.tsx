export type ModelColor = { name: string; hex: string }

type ColorSwatchesProps = {
  swatches: ModelColor[]
}

export function isModelColor(value: unknown): value is ModelColor {
  return (
    !!value &&
    typeof value === "object" &&
    "name" in value &&
    "hex" in value &&
    typeof value.name === "string" &&
    typeof value.hex === "string" &&
    /^#[0-9a-fA-F]{6}$/.test(value.hex)
  )
}

export default function ColorSwatches({ swatches }: ColorSwatchesProps) {
  if (!swatches.length) return null

  return (
    <div
      className="flex flex-wrap items-center gap-1.5"
      aria-label={`Model colours: ${swatches
        .map((swatch) => swatch.name)
        .join(", ")}`}
    >
      {swatches.map(({ name, hex }) => (
        <span
          key={name}
          className="inline-block h-5 w-5 rounded-full border border-black/20 shadow-sm"
          style={{ backgroundColor: hex }}
          title={name}
          aria-label={name}
        />
      ))}
    </div>
  )
}
