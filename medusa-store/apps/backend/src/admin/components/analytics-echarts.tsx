import { useEffect, useRef } from "react"
import { init, use } from "echarts/core"
import type { EChartsType } from "echarts/core"
import type { EChartsOption } from "echarts"
import { BarChart, LineChart, PieChart } from "echarts/charts"
import { AriaComponent, DataZoomComponent, GridComponent, LegendComponent, TooltipComponent } from "echarts/components"
import { LabelLayout } from "echarts/features"
import { SVGRenderer } from "echarts/renderers"
import { analyticsLocale, analyticsText } from "../lib/analytics-integration"

use([BarChart, LineChart, PieChart, AriaComponent, DataZoomComponent, GridComponent, LegendComponent, TooltipComponent, LabelLayout, SVGRenderer])

type ChartProps = {
  data?: Record<string, unknown>[]
  xAxisDataKey?: string
  yAxisDataKey?: string
  dataKey?: string
  dataKeys?: string[]
  yAxisTickFormatter?: (value: number) => string
  countryNames?: boolean
}

type ChartKind = "line" | "bar" | "pie" | "stacked"
const brandColors = ["#1677FF", "#0958D9", "#4096FF", "#69B1FF", "#91CAFF"]
const metricKeys: Record<string, string> = {
  count: "orderCount", sales: "totalSales", quantity: "quantitySold", total: "totalSales",
  new_customers: "newCustomers", returning_customers: "returningCustomers",
}

function AnalyticsChart({ kind, data = [], xAxisDataKey = "name", yAxisDataKey = "count", dataKey = "count", dataKeys, yAxisTickFormatter, countryNames }: ChartProps & { kind: ChartKind }) {
  const container = useRef<HTMLDivElement>(null)
  const locale = analyticsLocale()

  useEffect(() => {
    const element = container.current
    if (!element) return
    let chart: EChartsType | undefined
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)")
    const number = new Intl.NumberFormat(locale, { maximumFractionDigits: 2 })
    const formatValue = (value: unknown) => yAxisTickFormatter ? yAxisTickFormatter(Number(value)) : number.format(Number(value))
    const seriesName = (key: string) => analyticsText(metricKeys[key] || key)
    const draw = () => {
      if (!element.clientWidth || !element.clientHeight) return
      chart ||= init(element, undefined, { renderer: "svg", locale: locale === "zh-CN" ? "ZH" : "EN" })
      const dark = document.documentElement.classList.contains("dark")
      const text = dark ? "#FFFFFFD9" : "#000000A6"
      const divider = dark ? "#FFFFFF26" : "#0505050F"
      const compact = element.clientWidth < 460
      const countryDisplay = new Intl.DisplayNames([locale], { type: "region" })
      const names = data.map(row => {
        const name = String(row[xAxisDataKey] ?? "")
        if (!countryNames) return name
        return /^[A-Z]{2}$/.test(name) ? countryDisplay.of(name) || name : analyticsText("unknownCountry")
      })
      const keys = kind === "stacked" ? dataKeys || [] : [yAxisDataKey]
      const horizontal = kind === "bar"
      const category = {
        type: "category" as const,
        data: names,
        axisTick: { show: false },
        axisLine: { lineStyle: { color: divider } },
        axisLabel: { color: text, width: horizontal ? (compact ? 80 : 150) : 90, overflow: "truncate" as const, hideOverlap: true },
      }
      const valueAxis = {
        type: "value" as const,
        minInterval: yAxisTickFormatter ? undefined : 1,
        axisLabel: { color: text, formatter: formatValue },
        splitLine: { lineStyle: { color: divider } },
      }
      const option: EChartsOption = {
        color: brandColors,
        animation: !motion.matches,
        animationDuration: 250,
        textStyle: { fontFamily: "inherit", color: text },
        aria: { enabled: true, decal: { show: kind === "pie" || kind === "stacked" } },
        tooltip: {
          trigger: kind === "pie" ? "item" : "axis",
          renderMode: "richText",
          confine: true,
          backgroundColor: dark ? "#141414" : "#FFFFFF",
          borderColor: dark ? "#424242" : "#D9D9D9",
          textStyle: { color: dark ? "#FFFFFFD9" : "#000000E0" },
          valueFormatter: value => formatValue(value),
          axisPointer: { type: kind === "line" ? "line" : "shadow" },
        },
        legend: {
          show: kind === "pie" || kind === "stacked",
          type: "scroll",
          bottom: 0,
          textStyle: { color: text },
        },
      }
      if (kind === "pie") {
        option.series = [{
          type: "pie",
          radius: "62%",
          center: ["50%", "43%"],
          data: data.map(row => ({ name: analyticsText(`status_${String(row.name)}`), value: Number(row[dataKey]) || 0 })),
          label: { formatter: "{b}: {d}%", color: text, overflow: "truncate", width: compact ? 80 : 140 },
          labelLayout: { hideOverlap: true },
          emphasis: { scale: true },
        }]
      } else {
        option.grid = { top: 16, left: 8, right: 16, bottom: kind === "stacked" ? 48 : 12, containLabel: true }
        option.xAxis = horizontal ? valueAxis : category
        option.yAxis = horizontal ? { ...category, inverse: true } : valueAxis
        if (countryNames && data.length > 10) {
          option.grid = { ...option.grid, right: 40 }
          option.dataZoom = [
            { type: "slider", yAxisIndex: 0, right: 0, width: 12, startValue: 0, endValue: 9, showDetail: false },
            { type: "inside", yAxisIndex: 0, startValue: 0, endValue: 9, zoomOnMouseWheel: false, moveOnMouseWheel: true },
          ]
        }
        option.series = keys.map(key => ({
          name: seriesName(key),
          type: kind === "line" ? "line" : "bar",
          stack: kind === "stacked" ? "customers" : undefined,
          data: data.map(row => Number(row[key]) || 0),
          barMaxWidth: 32,
          symbol: "circle",
          symbolSize: data.length === 1 ? 8 : 5,
          showSymbol: data.length <= 31,
          emphasis: { focus: "series" },
        }))
      }
      chart.setOption(option, { notMerge: true })
      chart.resize()
    }
    draw()
    const resize = new ResizeObserver(draw)
    resize.observe(element)
    const theme = new MutationObserver(draw)
    theme.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
    motion.addEventListener("change", draw)
    return () => {
      resize.disconnect()
      theme.disconnect()
      motion.removeEventListener("change", draw)
      chart?.dispose()
    }
  }, [data, kind, xAxisDataKey, yAxisDataKey, dataKey, dataKeys, yAxisTickFormatter, countryNames, locale])

  return <div ref={container} data-analytics-echarts={kind} className="h-full min-w-0 w-full" />
}

export const AnalyticsLineChart = (props: ChartProps) => <AnalyticsChart {...props} kind="line" />
export const AnalyticsBarChart = (props: ChartProps) => <AnalyticsChart {...props} kind="bar" />
export const AnalyticsPieChart = (props: ChartProps) => <AnalyticsChart {...props} kind="pie" />
export const AnalyticsStackedBarChart = (props: ChartProps) => <AnalyticsChart {...props} kind="stacked" />
