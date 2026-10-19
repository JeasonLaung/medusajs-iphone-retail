import { forwardRef, useRef, type ComponentProps, type ComponentPropsWithoutRef } from "react"
import { IconButton, Input } from "@medusajs/ui"
import { useTranslation } from "react-i18next"

function clearField(field: HTMLInputElement | HTMLTextAreaElement | null) {
  if (!field || field.matches(":disabled") || field.readOnly) return
  const prototype = field instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(field, "")
  field.dispatchEvent(new Event("input", { bubbles: true }))
  field.focus()
}

function ClearButton({ onClear, inset = 0, multiline = false }: {
  onClear: () => void; inset?: number; multiline?: boolean
}) {
  const { t } = useTranslation()
  return (
    <IconButton type="button" size="small" variant="transparent"
      className={`absolute h-7 w-7 text-ui-fg-muted ${multiline ? "top-1" : "top-1/2 -translate-y-1/2"}`}
      style={{ right: inset + 4 }} aria-label={t("deviceScan.clearInput")} title={t("deviceScan.clearInput")}
      onMouseDown={(event) => event.preventDefault()}
      onClick={(event) => { event.preventDefault(); onClear() }}>
      <svg width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
        <path d="m5 5 10 10M15 5 5 15" strokeLinecap="round" />
      </svg>
    </IconButton>
  )
}

export const ClearableInput = forwardRef<HTMLInputElement, ComponentProps<typeof Input> & {
  medusa?: boolean; clearInset?: number
}>(({ medusa = false, clearInset = 0, className = "", size, ...props }, forwardedRef) => {
  const input = useRef<HTMLInputElement | null>(null)
  const hasValue = props.value != null && String(props.value).length > 0
  const fieldProps = { ...props, ref: (element: HTMLInputElement | null) => {
    input.current = element
    if (typeof forwardedRef === "function") forwardedRef(element)
    else if (forwardedRef) forwardedRef.current = element
  }, className: `w-full ${className}`, style: { ...props.style, paddingRight: clearInset + 36 } }
  return (
    <div className={`relative min-w-0 ${className.includes("flex-1") ? "flex-1" : "w-full"}`}>
      {medusa ? <Input {...fieldProps} size={size} /> : <input {...fieldProps} />}
      {hasValue && !props.disabled && !props.readOnly && <ClearButton inset={clearInset} onClear={() => clearField(input.current)} />}
    </div>
  )
})
ClearableInput.displayName = "ClearableInput"

export const ClearableTextarea = ({ className = "", ...props }: ComponentPropsWithoutRef<"textarea">) => {
  const input = useRef<HTMLTextAreaElement | null>(null)
  return (
    <div className="relative min-w-0 w-full">
      <textarea {...props} ref={input} className={className} style={{ ...props.style, paddingRight: 36 }} />
      {props.value != null && String(props.value).length > 0 && !props.disabled && !props.readOnly &&
        <ClearButton multiline onClear={() => clearField(input.current)} />}
    </div>
  )
}
