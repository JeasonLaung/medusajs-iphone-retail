import { optionValueLabel } from "@modules/products/i18n"

const titles: Record<string, { en: string; zh: string }> = {
  category: { en: "Series", zh: "系列" },
  model: { en: "Model", zh: "机型" },
  storage_options: { en: "Storage", zh: "容量" },
  battery_options: { en: "Battery", zh: "电池" },
  display_size: { en: "Display Size", zh: "屏幕尺寸" },
  color_families: { en: "Colour", zh: "颜色" },
  rear_cameras: { en: "Camera", zh: "相机" },
  condition_options: { en: "Grade", zh: "等级" },
  release_year: { en: "Release year", zh: "发布年份" },
}

export const getFilterActionsCopy = (segment: string) => segment?.endsWith("-zh")
  ? { heading: "筛选", clear: "清除筛选", open: "筛选与排序", close: "关闭筛选", showResults: "查看结果", cancel: "取消", confirm: "确认", sort: "排序", price: "价格", model: "机型", condition: "等级", all: "全部", toolbar: "商品排序与筛选" }
  : { heading: "Filter by", clear: "Clear filters", open: "Filter & sort", close: "Close filters", showResults: "Show results", cancel: "Cancel", confirm: "Confirm", sort: "Sort by", price: "Price", model: "Model", condition: "Grade", all: "All", toolbar: "Product sorting and filters" }

export const getSortLabel = (value: string, fallback: string, segment: string) => {
  if (!segment?.endsWith("-zh")) return fallback
  if (value.includes("created_at:desc")) return "最新上架"
  if (value.includes("min_price_") && value.endsWith(":asc")) return "价格从低到高"
  if (value.includes("min_price_") && value.endsWith(":desc")) return "价格从高到低"
  if (value.includes("title:asc")) return "名称 A → Z"
  if (value.includes("title:desc")) return "名称 Z → A"
  return "综合排序"
}

export const getRefinementTitle = (attribute: string, segment: string) =>
  titles[attribute]?.[segment?.endsWith("-zh") ? "zh" : "en"]

export const getCapacityNote = (segment: string) => segment?.endsWith("-zh")
  ? "容量按已录入的销售 SKU 筛选。电池、配色和成色仍是机型选项，不代表有对应现货。"
  : "Capacity filters use listed SKUs. Battery, colours and condition describe model options, not confirmed stock."

const colorLabels: Record<string, string> = {
  Black: "黑色",
  Blue: "蓝色",
  Gold: "金色",
  Gray: "灰色",
  Green: "绿色",
  Orange: "橙色",
  Pink: "粉色",
  Purple: "紫色",
  Red: "红色",
  Silver: "银色",
  White: "白色",
  Yellow: "黄色",
}

export const getRefinementLabel = (
  attribute: string,
  label: string,
  segment: string,
) =>
  attribute === "condition_options"
    ? optionValueLabel(label, segment)
    : attribute === "color_families" && segment?.endsWith("-zh")
    ? (colorLabels[label] ?? label)
    : label
