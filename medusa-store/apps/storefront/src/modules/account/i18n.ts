import en from "./translations/en.json"
import zh from "./translations/zh.json"

export type AccountKey = keyof typeof en
export const getAccountCopy = (locale: string | null | undefined) => {
  const chinese =
    !!locale &&
    (locale.toLowerCase().startsWith("zh") ||
      locale.toLowerCase().endsWith("-zh"))
  const copy = chinese ? zh : en
  return {
    locale: chinese ? "zh-CN" : "en-GB",
    t: (key: AccountKey, params: Record<string, string | number> = {}) =>
      Object.entries(params).reduce(
        (text, [name, value]) => text.replaceAll(`{${name}}`, String(value)),
        copy[key]
      ),
  }
}
