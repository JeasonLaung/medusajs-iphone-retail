"use client"

import { cloneElement, type AriaAttributes, type ReactElement } from "react"
import { Rate } from "antd"
import styles from "./styles.module.css"

export default function Rating({
  value,
  label,
  size = 20,
  showValue = true,
}: {
  value: number
  label: string
  size?: number
  showValue?: boolean
}) {
  return (
    <span role="img" aria-label={label} className={styles.display}>
      <span aria-hidden="true">
        <Rate disabled allowHalf value={value} style={{ fontSize: size }} />
      </span>
      {showValue && <span className={styles.value} aria-hidden="true">{value} / 5</span>}
    </span>
  )
}

export function RatingInput({
  value,
  onChange,
  disabled,
  label,
  starLabels,
}: {
  value: number
  onChange: (value: number) => void
  disabled: boolean
  label: string
  starLabels: string[]
}) {
  return (
    <div className={styles.input} role="group" aria-label={label}>
      <Rate
        value={value}
        onChange={onChange}
        allowClear={false}
        allowHalf={false}
        disabled={disabled}
        characterRender={(node, { index = 0 }) => {
          const star = node as ReactElement<{ children: ReactElement }>
          return cloneElement(star, {
            children: cloneElement(star.props.children, {
              "aria-label": starLabels[index],
              "aria-checked": value === index + 1,
            } as AriaAttributes),
          })
        }}
      />
      <span className={styles.value} aria-live="polite" aria-atomic="true">
        {starLabels[value - 1]}
      </span>
    </div>
  )
}
