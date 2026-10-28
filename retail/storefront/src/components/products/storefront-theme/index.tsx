import "@ant-design/v5-patch-for-react-19"
import { ConfigProvider, type ThemeConfig } from "antd"
import enGB from "antd/locale/en_GB.js"
import zhCN from "antd/locale/zh_CN.js"
import type { ReactNode } from "react"

const theme: ThemeConfig = {
  token: {
    fontFamily:
      '"DM Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", Arial, sans-serif',
    colorPrimary: "#1677FF",
    colorLink: "#0958D9",
    colorInfo: "#1677FF",
    colorSuccess: "#52C41A",
    colorWarning: "#FA8C16",
    colorError: "#F5222D",
    colorBgLayout: "#F8F8F8",
    colorBgContainer: "#FFFFFF",
    colorText: "#000000E0",
    colorTextSecondary: "#000000A6",
    colorBorder: "#D9D9D9",
    colorSplit: "#0505050F",
  },
}

export default function StorefrontTheme({
  locale,
  children,
}: {
  locale: string | null
  children: ReactNode
}) {
  return (
    <ConfigProvider
      theme={theme}
      locale={locale?.startsWith("zh") ? zhCN : enGB}
    >
      {children}
    </ConfigProvider>
  )
}
