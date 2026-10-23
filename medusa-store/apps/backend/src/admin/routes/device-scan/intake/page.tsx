import { defineRouteConfig } from "@medusajs/admin-sdk"
import DeviceWorkbench from "../../../components/device-workbench"
import { Button, Heading, Tabs } from "@medusajs/ui"
import { Link, useSearchParams } from "react-router-dom"
import { useState } from "react"
import { useDeviceTranslations } from "../../../lib/use-device-translations"

const DeviceIntakePage = () => {
  const { t } = useDeviceTranslations()
  const [params] = useSearchParams()
  const orderId = params.get("order_id")
  const returnTo = `/orders/${encodeURIComponent(orderId || "")}`
  const [method, setMethod] = useState<"image" | "scan" | "import">("scan")
  const [busy, setBusy] = useState(false)
  return (
    <div className="space-y-4">
      <Heading level="h1">{t("deviceScan.intake")}</Heading>
      {orderId && (
        <Button size="small" variant="secondary" asChild disabled={busy}>
          <Link
            to={returnTo}
            onClick={(event) => {
              if (busy) event.preventDefault()
            }}
          >
            {t("orderDevices.backToAllocation")}
          </Link>
        </Button>
      )}
      <Tabs
        value={method}
        onValueChange={(value) => {
          if (
            !busy &&
            (value === "image" || value === "scan" || value === "import")
          )
            setMethod(value)
        }}
      >
        <Tabs.List aria-label={t("deviceScan.intakeMethod")}>
          <Tabs.Trigger value="image" disabled={busy}>
            {t("deviceScan.lookupImageIntake")}
          </Tabs.Trigger>
          <Tabs.Trigger value="scan" disabled={busy}>
            {t("deviceScan.lookupScanIntake")}
          </Tabs.Trigger>
          <Tabs.Trigger value="import" disabled={busy}>
            {t("deviceScan.lookupImportIntake")}
          </Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value={method} className="pt-4">
          <DeviceWorkbench
            key={method}
            mode="intake"
            intakeMethod={method}
            onBusyChange={setBusy}
          />
        </Tabs.Content>
      </Tabs>
    </div>
  )
}

export default DeviceIntakePage

export const config = defineRouteConfig({
  label: "deviceScan.intake",
  translationNs: "translation",
  rank: 3,
})
