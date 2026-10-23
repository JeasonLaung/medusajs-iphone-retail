import type { ReactNode } from "react"
import { App, ConfigProvider } from "antd"
import enUS from "antd/locale/en_US"
import zhCN from "antd/locale/zh_CN"
import { useTranslation } from "react-i18next"
import Operations from "./Operations"
import CrawlerLogs from "./CrawlerLogs"
import TrafficSources from "./TrafficSources"
import Providers from "./Providers"
import CitationMonitor from "./CitationMonitor"
import PageMetas from "./PageMetas"
import SEOSettings from "./SEOSettings"
import "./reference.css"

export type SEOSection = "overview" | "crawler-logs" | "traffic-sources" | "providers" | "citation-monitor" | "page-metas" | "settings"
const sections: Record<SEOSection, () => ReactNode> = { overview: Operations, "crawler-logs": CrawlerLogs, "traffic-sources": TrafficSources, providers: Providers, "citation-monitor": CitationMonitor, "page-metas": PageMetas, settings: SEOSettings }
export default function SEOReference({ section = "overview" }: { section?: SEOSection }) {
  const { i18n } = useTranslation(), Section = sections[section]
  return <ConfigProvider locale={i18n.language === "zhCN" ? zhCN : enUS} theme={{ token: {
    colorPrimary: "#1677FF", colorInfo: "#1677FF", colorSuccess: "#52C41A", colorWarning: "#FA8C16", colorError: "#F5222D",
    colorBgLayout: "#F8F8F8", colorBgContainer: "#FFFFFF", colorText: "#000000E0", colorTextSecondary: "#000000A6", colorBorder: "#D9D9D9", colorSplit: "#0505050F", borderRadius: 6,
    fontFamily: "Inter, ui-sans-serif, system-ui, sans-serif", motion: !window.matchMedia("(prefers-reduced-motion: reduce)").matches,
  } }}><App key={i18n.language} className="seo-reference"><Section /></App></ConfigProvider>
}
