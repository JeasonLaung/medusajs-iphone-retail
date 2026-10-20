import { Button, Text } from "@medusajs/ui"
import { Link } from "react-router-dom"
import { useDeviceTranslations } from "../lib/use-device-translations"

export default function DeviceIntakeShortcut({
  orderId,
  busy,
  fromWorkbench = false,
}: {
  orderId: string
  busy: boolean
  fromWorkbench?: boolean
}) {
  const { t } = useDeviceTranslations()
  const params = new URLSearchParams({ order_id: orderId })
  if (fromWorkbench) params.set("return_to", "workbench")
  return (
    <div className="space-y-2">
      <Text size="small">{t("orderDevices.stockOnlyHelp")}</Text>
      <Button size="small" variant="secondary" asChild disabled={busy}>
        <Link
          to={`/device-scan/intake?${params}`}
          onClick={(event) => {
            if (busy) event.preventDefault()
          }}
        >
          {t("orderDevices.goToIntake")}
        </Link>
      </Button>
    </div>
  )
}
