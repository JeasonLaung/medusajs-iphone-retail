import en from "../admin/i18n/json/en.json"
import zhCN from "../admin/i18n/json/zh-CN.json"

export const deviceImportKeys = ["serial_number", "imei", "imei2", "eid", "sku_parameters", "sku", "grade", "battery_health", "cost_eur", "currency_code", "bin", "notes"] as const

export type TemplateSku = { sku?: string | null; title?: string; product?: { title?: string } | null; options?: { value: string; option?: { title?: string } | null }[] | null }

export function templateSkuParameters(variant: TemplateSku) {
  const parameters = variant.options?.length ? variant.options.map((option) => option.value).join(" / ") : variant.title
  return [variant.product?.title, parameters, `[${variant.sku}]`].filter(Boolean).join("/").replace(/\s+/g, "")
}

export function templateSkuFormula(row: number, lastSourceRow: number, parameterColumn = "E") {
  const input = `$${parameterColumn}${row}`
  const exact = `SUBSTITUTE(SUBSTITUTE(SUBSTITUTE(${input},"~","~~"),"*","~*"),"?","~?")`
  return `IF(${input}="","",IFERROR(INDEX('SKU source'!$A$2:$A$${lastSourceRow},MATCH(${exact},'SKU source'!$F$2:$F$${lastSourceRow},0)),""))`
}

export const deviceImportHeaders = (locale = "en") =>
  locale.toLowerCase().startsWith("zh") ? zhCN.deviceScan.templateHeaders : en.deviceScan.templateHeaders

const normalizeHeader = (value: string) => value.normalize("NFKC").trim().toLowerCase().replace(/[\s_\-()/%]/g, "")

const aliases: Record<string, string[]> = {
  serial_number: ["Serial No.", "Serial No", "S/N", "SN", "设备序列号"],
  imei: ["IMEI 1", "第一IMEI"],
  imei2: ["第二IMEI"],
  grade: ["Condition", "Condition Grade", "成色等级"],
  battery_health: ["Battery Health (%)", "电池健康度（%）"],
  cost_eur: ["Cost EUR", "Purchase Cost (EUR)", "采购成本（欧元）", "成本（欧元）"],
  currency_code: ["Currency Code", "货币代码"],
  bin: ["Bin", "Bin Location", "库位"],
  notes: ["Remarks"],
  variant_id: ["Variant ID", "规格ID"],
}

const headerKeys = new Map<string, string>()
for (const key of [...deviceImportKeys, "variant_id"] as const) {
  const labels = key === "variant_id" ? [] : [deviceImportHeaders("en")[key], deviceImportHeaders("zhCN")[key]]
  for (const label of [key, ...labels, ...(aliases[key] || [])]) headerKeys.set(normalizeHeader(label), key)
}

export const matchDeviceImportHeader = (value: string) => headerKeys.get(normalizeHeader(value))
