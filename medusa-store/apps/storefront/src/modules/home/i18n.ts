const en = {
  metaTitle: "iPhone | Explore the series",
  eyebrow: "THE IPHONE COLLECTION",
  title: "iPhone series",
  description: "Explore the collection, one series at a time.",
  allModels: "Shop all iPhones",
  series: "series",
  seriesName: "{name} series",
  explore: "Explore series",
  modelCount: "{count} models",
  oneModel: "1 model",
  imageAlt: "{name} series",
  empty: "Our iPhone collection is being updated. Please check back soon.",
}

const zh: typeof en = {
  metaTitle: "iPhone | 探索各个系列",
  eyebrow: "IPHONE 系列精选",
  title: "iPhone 系列",
  description: "从喜爱的系列开始，探索适合你的机型。",
  allModels: "查看全部 iPhone",
  series: "系列",
  seriesName: "{name} 系列",
  explore: "探索系列",
  modelCount: "{count} 款机型",
  oneModel: "1 款机型",
  imageAlt: "{name} 系列",
  empty: "iPhone 商品正在更新，请稍后再来看看。",
}

export type HomeCopy = typeof en
export const getHomeCopy = (locale: string | null): HomeCopy =>
  locale?.toLowerCase().startsWith("zh") ? zh : en
