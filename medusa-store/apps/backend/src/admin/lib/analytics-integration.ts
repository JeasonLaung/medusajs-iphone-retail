import { useEffect } from "react"
import { getI18n, useTranslation } from "react-i18next"
import { notifyError } from "./notifications"
import en from "../i18n/json/analytics-en.json"
import zh from "../i18n/json/analytics-zh.json"

let analyticsI18n: ReturnType<typeof getI18n> | undefined

export function analyticsText(key: string, values?: Record<string, unknown>) {
  return (analyticsI18n || getI18n())?.t(`analytics.${key}`, { defaultValue: en[key as keyof typeof en] || key, ...values }) || en[key as keyof typeof en] || key
}

export function analyticsLocale() {
  return (analyticsI18n || getI18n())?.language?.startsWith("zh") ? "zh-CN" : "en-US"
}

export function useAnalyticsLocale() {
  const { i18n } = useTranslation()
  analyticsI18n = i18n
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { analytics: en }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { analytics: zh }, true, true)
  }, [i18n])
  return i18n.language.startsWith("zh") ? "zh-CN" : "en-US"
}

export function useAnalyticsErrors(error: unknown) {
  useEffect(() => {
    if (error) notifyError(analyticsText("loadError"))
  }, [error])
}
