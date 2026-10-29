const copy = {
  en: { storage: "Model capacities", modelColours: "Model colours", noSku: "No matching SKU", outOfStock: "Out of stock", view: "View", fromPrice: "From {price}" },
  zh: { storage: "机型容量", modelColours: "机型配色", noSku: "暂无对应 SKU", outOfStock: "暂无库存", view: "查看", fromPrice: "{price} 起" },
}

export const getProductCardCopy = (segment: string) =>
  copy[segment?.endsWith("-zh") ? "zh" : "en"]
