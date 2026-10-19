// Each space-separated term narrows the same variant. Option conditions use
// separate EXISTS clauses so capacity, battery and condition may match different rows.
const colors: Record<string, string> = {
  黑: "black", 黑色: "black", 白: "white", 白色: "white",
  蓝: "blue", 蓝色: "blue", 绿: "green", 绿色: "green",
  紫: "purple", 紫色: "purple", 粉: "pink", 粉色: "pink",
  红: "red", 红色: "red", 黄: "yellow", 黄色: "yellow",
  金: "gold", 金色: "gold", 银: "silver", 银色: "silver",
  橙: "orange", 橙色: "orange", 石墨: "graphite", 石墨色: "graphite",
  午夜色: "midnight", 星光色: "starlight", 深空黑: "space black",
  暗紫色: "deep purple", 远峰蓝: "sierra blue", 苍岭绿: "alpine green",
  海蓝色: "pacific blue", 原色钛金属: "natural titanium",
  黑色钛金属: "black titanium", 白色钛金属: "white titanium",
  蓝色钛金属: "blue titanium", 沙漠色钛金属: "desert titanium",
}

function contains(value: string) {
  return { $ilike: `%${value.replace(/[\\%_]/g, "\\$&")}%` }
}

function option(value: string, exact = false) {
  return { options: { $some: { value: exact ? value : contains(value) } } }
}

export function deviceVariantSearchFilters(search: string): Record<string, unknown> {
  const normalized = search.normalize("NFKC").trim().toLowerCase()
    .replace(/\b(?:iphone|i)\s*(\d{1,2})\s*(pro\s*max|pm|pro|p|plus|mini|m)?\b/g,
      (_, generation: string, suffix: string | undefined) => `i${generation}${suffix?.replace(/\s/g, "") || ""} `)
  const terms = normalized.split(/\s+/).filter(Boolean)
  const conditions = terms.map((term) => {
    const model = term.match(/^(?:i|iphone)(\d{1,2})(pm|promax|p|pro|plus|mini|m)?$/)
    if (model) {
      const suffix: Record<string, string> = {
        pm: " Pro Max", promax: " Pro Max", p: " Pro", pro: " Pro",
        plus: " Plus", mini: " Mini", m: " Mini",
      }
      return { product: { title: { $ilike: `iPhone ${model[1]}${suffix[model[2]] || ""}` } } }
    }
    const grade = term.match(/^([sabcd])(?:级)?$/)
    if (grade) return option(grade[1].toUpperCase(), true)
    const capacity = term.match(/^(\d+)(g|gb|t|tb)$/)
    if (capacity) {
      const size = Number(capacity[1]) * (capacity[2].startsWith("t") ? 1024 : 1)
      return option(`${size} GB`, true)
    }
    if (/^\d+%$/.test(term) || /^(85|100)$/.test(term)) {
      return option(`${parseInt(term, 10)}%`, true)
    }
    if (/^(64|128|256|512|1024|2048)$/.test(term)) return option(`${term} GB`, true)
    const value = colors[term] || term
    return {
      $or: [
        { sku: contains(value) },
        { title: contains(value) },
        { product: { title: contains(value) } },
        option(value),
      ],
    }
  })
  return conditions.length ? { $and: conditions } : {}
}
