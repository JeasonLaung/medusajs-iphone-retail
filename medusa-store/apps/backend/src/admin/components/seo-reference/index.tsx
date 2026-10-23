import { useEffect, useState, type CSSProperties, type ReactNode } from "react"
import { App, ConfigProvider, theme } from "antd"
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
function SEOThemeSurface({ children }: { children: ReactNode }) {
  const { token } = theme.useToken()
  const style = {
    "--seo-text": token.colorText,
    "--seo-secondary": token.colorTextSecondary,
    "--seo-border": token.colorBorder,
    "--seo-divider": token.colorSplit,
    "--seo-surface": token.colorFillAlter,
    "--seo-success": token.colorSuccess,
    "--seo-success-bg": token.colorSuccessBg,
    "--seo-warning": token.colorWarning,
    "--seo-warning-bg": token.colorWarningBg,
    "--seo-primary": token.colorPrimary,
  } as CSSProperties
  return <App className="seo-reference" style={style}>{children}</App>
}

export default function SEOReference({ section = "overview" }: { section?: SEOSection }) {
  const { i18n } = useTranslation(), Section = sections[section]
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"))
  useEffect(() => {
    const syncTheme = () => setDark(document.documentElement.classList.contains("dark"))
    const observer = new MutationObserver(syncTheme)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
    syncTheme()
    return () => observer.disconnect()
  }, [])
  return <ConfigProvider locale={i18n.language === "zhCN" ? zhCN : enUS} theme={{
    algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
    token: {
      colorPrimary: "#1677FF", colorInfo: "#1677FF", colorSuccess: "#52C41A", colorWarning: "#FA8C16", colorError: "#F5222D",
      ...(!dark && { colorBgLayout: "#F8F8F8", colorBgContainer: "#FFFFFF", colorText: "#000000E0", colorTextSecondary: "#000000A6", colorBorder: "#D9D9D9", colorSplit: "#0505050F", colorFillAlter: "#F8F8F8" }),
      borderRadius: 6,
      fontFamily: "Inter, ui-sans-serif, system-ui, sans-serif", motion: !window.matchMedia("(prefers-reduced-motion: reduce)").matches,
    },
  }}><SEOThemeSurface key={i18n.language}><Section /></SEOThemeSurface></ConfigProvider>
}
