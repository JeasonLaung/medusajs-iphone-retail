import { useState, type ComponentProps } from "react"
import { IconButton, Input, Popover } from "@medusajs/ui"
import { useTranslation } from "react-i18next"
import DeviceScanner from "./device-scanner"
import { ClearableInput } from "./clearable-input"

export default function DeviceIdentityInput({
  scanLabel,
  onScan,
  onScanMany,
  recognizeSku,
  ...props
}: ComponentProps<typeof Input> & {
  scanLabel: string
  onScan: (value: string) => void
  onScanMany?: (values: string[], skuSearch?: string) => void
  recognizeSku?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const label = t("orderDevices.scanField", { field: scanLabel })

  return (
    <Popover open={open && !props.disabled} onOpenChange={setOpen}>
      <div className="relative">
        <ClearableInput {...props} medusa clearInset={36} />
        <Popover.Trigger asChild>
          <IconButton
            type="button"
            size="small"
            variant="transparent"
            disabled={props.disabled}
            className="absolute right-0 top-0 h-8 w-9 rounded-l-none border-l border-ui-border-base"
            aria-label={label}
            title={label}
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M8 3H4a1 1 0 0 0-1 1v4m13-5h4a1 1 0 0 1 1 1v4M3 16v4a1 1 0 0 0 1 1h4m8 0h4a1 1 0 0 0 1-1v-4M7 8v8m3-8v8m4-8v8m3-8v8" />
            </svg>
          </IconButton>
        </Popover.Trigger>
      </div>
      <Popover.Content
        align="end"
        className="max-h-[85vh] w-80 max-w-[calc(100vw-2rem)] overflow-y-auto p-0"
        aria-label={label}
      >
        <DeviceScanner
          autoStart
          showManualInput={false}
          recognizeSku={recognizeSku}
          onScanMany={onScanMany ? (values, skuSearch) => {
            onScanMany(values, skuSearch)
            setOpen(false)
          } : undefined}
          onScan={(value) => {
            onScan(value.trim().toUpperCase())
            setOpen(false)
          }}
        />
      </Popover.Content>
    </Popover>
  )
}
