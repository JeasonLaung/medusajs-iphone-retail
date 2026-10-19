// Only recognized product attributes become search terms. Identity numbers and
// other label text must never narrow the SKU list accidentally.
export function deviceLabelSearch(text: string): string {
  const normalized = text.normalize("NFKC").toLowerCase()
    .replace(/\bi\s*phone\b/g, "iphone")
  const models = [...new Set([...normalized.matchAll(/\biphone\s*(\d{1,2}|se|air)\b(?:\s*(pro\s*max|pro|plus|mini|air)\b)?/g)]
    .map((match) => `iphone ${match[1]}${match[2] ? ` ${match[2].replace(/\s+/g, " ")}` : ""}`))]
  if (models.length !== 1) return ""
  const capacities = [...new Set([...normalized.matchAll(/\b(32|64|128|256|512|1024|2048|1|2)\s*(gb|g|tb|t)\b/g)]
    .map((match) => String(Number(match[1]) * (match[2].startsWith("t") ? 1024 : 1))))]
  const colors = [...new Set([...normalized.matchAll(/\b(natural titanium|black titanium|white titanium|blue titanium|desert titanium|space black|space gr[ae]y|deep purple|sierra blue|alpine green|pacific blue|rose gold|graphite|midnight|starlight|yellow|black|white|blue|green|purple|pink|red|gold|silver|orange|teal|ultramarine)\b/g)]
    .map((match) => match[1].replace("grey", "gray")))]
  return [models[0], capacities.length === 1 ? capacities[0] : "", colors.length === 1 ? colors[0] : ""]
    .filter(Boolean).join(" ")
}
