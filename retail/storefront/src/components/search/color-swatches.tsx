export type ModelColor = { name: string; hex: string }

type ColorSwatchesProps = {
  swatches: (ModelColor & { disabled?: boolean })[]
  unavailableLabel?: string
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

export default function ColorSwatches({ swatches, unavailableLabel }: ColorSwatchesProps) {
  if (!swatches.length) return null

  return (
    <div
      className="flex flex-wrap items-center gap-1.5"
      aria-label={`Model colours: ${swatches
        .map((swatch) => swatch.name)
        .join(", ")}`}
    >
      {swatches.map(({ name, hex, disabled }) => (
        <span
          key={name}
          className={`relative inline-block h-5 w-5 rounded-full border border-black/20 shadow-sm${disabled ? " opacity-40" : ""}`}
          style={{ backgroundColor: hex }}
          title={disabled && unavailableLabel ? `${name}: ${unavailableLabel}` : name}
          aria-label={name}
          aria-disabled={disabled || undefined}
          data-disabled={disabled || undefined}
          data-testid="product-color"
        >
          {disabled && <span aria-hidden="true" className="absolute -left-[8%] top-1/2 h-0.5 w-[116%] -rotate-45 bg-white shadow-[0_0_0_1px_#9b9ba3]" />}
        </span>
      ))}
    </div>
  )
}
