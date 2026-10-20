import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Button, Input, Text } from "@medusajs/ui"
import QRCode from "qrcode"
import { createMobileIntakeLink } from "../lib/mobile-intake"
import { notifyError } from "../lib/notifications"

export default function MobileIntakeLink({ sourceId, enabled, category = "sale", autoCreate = false, onBusyChange }: { sourceId: string; enabled: boolean; category?: "sale" | "returned" | "recycled"; autoCreate?: boolean; onBusyChange?: (busy: boolean) => void }) {
  const { t, i18n } = useTranslation()
  const [origin, setOrigin] = useState(window.location.origin)
  const [link, setLink] = useState("")
  const [qr, setQr] = useState("")
  const [busy, setBusy] = useState(false)
  const [expires, setExpires] = useState(0)
  const [copied, setCopied] = useState(false)
  useEffect(() => { onBusyChange?.(busy) }, [busy, onBusyChange])
  useEffect(() => {
    if (autoCreate && enabled && sourceId) void createLink()
  }, [autoCreate, enabled, sourceId, category, i18n.language])

  async function createLink() {
    setBusy(true)
    setCopied(false)
    setLink("")
    setQr("")
    try {
      const result = await createMobileIntakeLink(sourceId, category, i18n.language, origin)
      const value = result.url
      const image = await QRCode.toDataURL(value, { width: 280, margin: 2, errorCorrectionLevel: "M" })
      setExpires(result.expiresAt)
      setLink(value)
      setQr(image)
    } catch (error) { notifyError(error instanceof TypeError || (error instanceof Error && error.message === "mobileIntake.originError") ? t("mobileIntake.originError") : t("mobileIntake.failed")) }
    finally { setBusy(false) }
  }

  return <div className="space-y-3 rounded-lg border p-4">
    <Text weight="plus">{t("mobileIntake.title")}</Text>
    <Text size="small" className="text-ui-fg-subtle">{t("mobileIntake.help")}</Text>
    <label className="block space-y-1">
      <Text size="small">{t("mobileIntake.origin")}</Text>
      <Input value={origin} onChange={(event) => { setOrigin(event.target.value); setLink(""); setQr("") }} placeholder="https://inventory.example.com" />
    </label>
    <Button size="small" variant="secondary" disabled={!enabled || busy} isLoading={busy} onClick={createLink}>{t("mobileIntake.create")}</Button>
    {link && <div className="space-y-2">
      <img src={qr} width={280} height={280} alt={t("mobileIntake.qr")} className="max-w-full" />
      <Text size="small">{t("mobileIntake.expires", { time: new Date(expires).toLocaleString(i18n.language === "zhCN" ? "zh-CN" : "en") })}</Text>
      <Input aria-label={t("mobileIntake.link")} value={link} readOnly onFocus={(event) => event.target.select()} />
      <div className="flex gap-2">
        <Button size="small" variant="secondary" onClick={async () => {
          try { await navigator.clipboard.writeText(link); setCopied(true) }
          catch { notifyError(t("mobileIntake.copyError")) }
        }}>{t(copied ? "mobileIntake.copied" : "mobileIntake.copy")}</Button>
        <a href={link} target="_blank" rel="noreferrer" className="text-sm text-ui-fg-interactive underline">{t("mobileIntake.open")}</a>
      </div>
    </div>}
  </div>
}
