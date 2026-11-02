// Source: medusa-store/apps/storefront/src/modules/common/components/grade-badge/index.tsx
import { optionValueLabel } from "../../products/i18n"
import styles from "./grade-badge.module.css"

export const isGrade = (value: string) => ["S", "A", "B", "C"].includes(value)

export default function GradeBadge({ grade, locale }: { grade: string; locale: string }) {
  return <span className={styles.badge} data-grade={grade}>{optionValueLabel(grade, locale)}</span>
}
