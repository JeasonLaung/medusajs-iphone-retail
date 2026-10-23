import fs from "node:fs"
import path from "node:path"
import { MedusaError } from "@medusajs/framework/utils"
import type { DepOptimizationOptions, Plugin } from "vite" with { "resolution-mode": "import" }
import en from "../admin/i18n/json/analytics-en.json"

// Adapt the pinned upstream bundle without changing installed package files.
export function transformAnalytics(source: string, id: string) {
  const file = id.replace(/\\/g, "/").split("?")[0]
  if (!file.includes("/@agilo/medusa-analytics-plugin/") || !file.endsWith("/src/admin/index.mjs")) return
  if (!source.includes("const AnalyticsPage = () => {") || !source.includes("Component: AnalyticsPage,")) {
    throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Analytics 1.4.0 integration: upstream page structure changed")
  }
  const helper = path.resolve(__dirname, "../admin/lib/analytics-integration.ts").replace(/\\/g, "/")
  const charts = path.resolve(__dirname, "../admin/components/analytics-echarts.tsx").replace(/\\/g, "/")
  // Replace the four upstream chart components, retaining page queries and tables.
  const chartAnchors = ["const useDarkMode =", "function cn(", "const BarChart =", "const columnHelper$3 =", "const StackedBarChart =", "const dummyData =", "data: orders.regions,"]
  if (chartAnchors.some(anchor => !source.includes(anchor))) {
    throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Analytics 1.4.0 integration: upstream chart structure changed")
  }
  let code = source
    .replace(/^import .* from "recharts";\r?\n/m, "")
    .replace(/const useDarkMode =[\s\S]*?(?=function cn\()/, "")
    .replace(/function generateStableColor\([\s\S]*?(?=const sdk =)/, "")
    .replace(/const BarChart =[\s\S]*?(?=const columnHelper\$3 =)/, "")
    .replace(/const StackedBarChart =[\s\S]*?(?=const dummyData =)/, "")
    .replaceAll('style: { aspectRatio: "16/9" }', 'style: { height: "320px", minWidth: 0 }')
    .replace('data: orders.regions,', 'countryNames: true, data: orders.regions,')
    .replace('"Top Regions by Sales"', 'analyticsText("topRegions")')
    .replace('"Sales breakdown by region in the selected period"', 'analyticsText("regionsDescription")')
    .replace('label: "Analytics",', 'label: "analytics.title",')
    .replace("translationNs: void 0", 'translationNs: "translation"')
    .replace("import { DateRangePicker,", "import { I18nProvider, DateRangePicker,")
    .replace("Component: AnalyticsPage,", "Component: () => { const locale = useAnalyticsLocale(); return jsx(I18nProvider, { locale, children: jsx(AnalyticsPage, { key: locale }) }); },")
    .replace("data: products, isLoading: isLoadingProducts", "data: products, error: productError, isLoading: isLoadingProducts")
    .replace("data: customers, isLoading: isLoadingCustomers", "data: customers, error: customerError, isLoading: isLoadingCustomers")
    .replace("data: orders, isLoading: isLoadingOrders", "data: orders, error: orderError, isLoading: isLoadingOrders")
    .replace("  const someOrderCountsGreaterThanZero", "  useAnalyticsErrors(orderError || productError || customerError);\n  const someOrderCountsGreaterThanZero")
    .replace(/new Intl.NumberFormat\("en-US"/g, "new Intl.NumberFormat(analyticsLocale()")
    .replace('format(new Date(date), "MMM dd, yyyy")', 'new Intl.DateTimeFormat(analyticsLocale(), { dateStyle: "medium" }).format(new Date(date))')
    .replace(/"Total sales in the selected period \(",\s*orders == null \? void 0 : orders.currency_code,\s*"\)"/, 'analyticsText("salesDescription", { currency: orders?.currency_code || "EUR" })')
  for (const [key, value] of Object.entries(en)) {
    if (key === "salesDescription") continue
    const literal = JSON.stringify(value)
    for (const prop of ["header", "sortLabel", "sortAscLabel", "sortDescLabel"]) {
      code = code.replaceAll(`${prop}: ${literal}`, `get ${prop}() { return analyticsText(${JSON.stringify(key)}); }`)
    }
    code = code.replaceAll(literal, `analyticsText(${JSON.stringify(key)})`)
  }
  return `import { AnalyticsLineChart as LineChart, AnalyticsBarChart as BarChart, AnalyticsPieChart as PieChart, AnalyticsStackedBarChart as StackedBarChart } from ${JSON.stringify(charts)}\nimport { analyticsText, analyticsLocale, useAnalyticsLocale, useAnalyticsErrors } from ${JSON.stringify(helper)}\n${code}`
}

export const adminAnalytics = (): Plugin => ({
  name: "admin-analytics-integration",
  enforce: "pre",
  transform(source, id) {
    const code = transformAnalytics(source, id)
    if (code) return { code, map: null }
  },
})

type OptimizationPlugin = NonNullable<NonNullable<DepOptimizationOptions["esbuildOptions"]>["plugins"]>[number]
export const adminAnalyticsOptimizer = (): OptimizationPlugin => ({
  name: "admin-analytics-optimizer-countries-v1",
  setup(build) {
    build.onResolve({ filter: /\/src\/admin\/components\/analytics-echarts\.tsx$/ }, (args) => ({ path: `/@fs/${args.path}`, external: true }))
    build.onResolve({ filter: /\/src\/admin\/lib\/analytics-integration\.ts$/ }, (args) => ({ path: `/@fs/${args.path}`, external: true }))
    build.onLoad({ filter: /index\.mjs$/ }, (args) => {
      const normalized = args.path.replace(/\\/g, "/")
      if (!normalized.includes("/@agilo/medusa-analytics-plugin/")) return
      const contents = transformAnalytics(fs.readFileSync(args.path, "utf8"), args.path)
      if (contents) return { contents, loader: "js", resolveDir: path.dirname(args.path) }
    })
  },
})
