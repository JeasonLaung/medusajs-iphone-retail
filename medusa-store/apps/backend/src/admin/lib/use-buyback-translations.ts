import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import en from "../i18n/json/buyback-en.json"
import zh from "../i18n/json/buyback-zh.json"

export function useBuybackTranslations() {
  const value = useTranslation(undefined, { bindI18n: "languageChanged added" })
  useEffect(() => {
    value.i18n.addResourceBundle("en", "translation", { buyback: en }, true, true)
    value.i18n.addResourceBundle("zhCN", "translation", { buyback: zh }, true, true)
  }, [value.i18n, en, zh])
  return value
}
