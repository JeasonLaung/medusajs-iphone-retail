import { optionValueLabel } from "../i18n"
import styles from "./styles.module.css"

export const isGrade = (value: string) => ["S", "A", "B", "C"].includes(value)

export default function GradeBadge({ grade, locale }: { grade: string; locale: string }) {
  return <span className={styles.badge} data-grade={grade}>{optionValueLabel(grade, locale)}</span>
}
