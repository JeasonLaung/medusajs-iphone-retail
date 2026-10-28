import { theme } from "antd"
import type { CSSProperties, InputHTMLAttributes, ReactNode } from "react"
import styles from "./styles.module.css"

type SelectionRowProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "children"> & {
  type?: "radio" | "checkbox"
  children: ReactNode
  trailing?: ReactNode
}

export default function SelectionRow({ type = "radio", children, trailing, className, ...props }: SelectionRowProps) {
  const { token } = theme.useToken()

  return <label className={styles.row} style={{
    "--choice-surface": token.colorBgContainer,
    "--choice-hover": token.colorBgLayout,
    "--choice-selected": token.colorPrimaryBg,
    "--choice-brand": token.colorPrimary,
    "--choice-text": token.colorText,
    "--choice-disabled": token.colorTextDisabled,
  } as CSSProperties}>
    <input {...props} type={type} className={className} />
    <span className={styles.label}>{children}</span>
    {trailing != null && <span className={styles.trailing}>{trailing}</span>}
  </label>
}
