import { Button, DropdownMenu } from "@medusajs/ui"
import { useDeviceTranslations } from "../lib/use-device-translations"

export default function ScanIntakeMenu({ onDirect, onQr, busy = false }: {
  onDirect: () => void
  onQr: () => void
  busy?: boolean
}) {
  const { t } = useDeviceTranslations()
  return <DropdownMenu>
    <DropdownMenu.Trigger asChild>
      <Button size="small" disabled={busy} isLoading={busy}>
        {t("deviceScan.lookupScanIntake")}
        <svg width="16" height="16" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <path d="m6 8 4 4 4-4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </Button>
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="end">
      <DropdownMenu.Item onSelect={onDirect}>{t("deviceScan.scanIntakeDirect")}</DropdownMenu.Item>
      <DropdownMenu.Item onSelect={onQr}>{t("deviceScan.scanIntakeQr")}</DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu>
}
