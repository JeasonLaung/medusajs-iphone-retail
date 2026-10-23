import { getI18n } from "react-i18next"
import common from "./en.json"
import extra from "./en-extra.json"
const dictionary: Record<string, string> = { ...common, ...extra }
const fragments = Object.keys(dictionary).sort((a, b) => b.length - a.length)
const expression = new RegExp(fragments.map(text => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|"), "g")
export default function tr(value: string): string {
  if (getI18n()?.language?.startsWith("zh")) return value
  if (dictionary[value] !== undefined) return dictionary[value]
  return value.replace(expression, text => dictionary[text])
}
