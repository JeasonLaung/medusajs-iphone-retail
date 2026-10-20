import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import en from "../i18n/json/en.json"
import zhCN from "../i18n/json/zh-CN.json"

const translationOptions = { bindI18n: "languageChanged", bindI18nStore: "added" }

// The dashboard retains its i18n instance during hot updates. Refresh only our
// custom resources so newly added labels are available without restarting it.
export function useDeviceTranslations() {
  const translation = useTranslation(undefined, translationOptions)
  const { i18n } = translation
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { deviceScan: en.deviceScan, orderDevices: en.orderDevices }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { deviceScan: zhCN.deviceScan, orderDevices: zhCN.orderDevices }, true, true)
  }, [i18n, en, zhCN])
  return translation
}
