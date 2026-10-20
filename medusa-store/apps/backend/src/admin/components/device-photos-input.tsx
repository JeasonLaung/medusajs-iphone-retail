import { Button, Text } from "@medusajs/ui"
import { useId } from "react"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { useDeviceTranslations } from "../lib/use-device-translations"

export type DevicePhoto = { id: string; url: string }

async function photoContent(file: File) {
  const url = URL.createObjectURL(file)
  try {
    const image = new Image()
    image.src = url
    await image.decode()
    const scale = Math.min(1, 1800 / Math.max(image.width, image.height))
    const canvas = document.createElement("canvas")
    canvas.width = Math.round(image.width * scale)
    canvas.height = Math.round(image.height * scale)
    const context = canvas.getContext("2d")!
    context.fillStyle = "#fff"
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    return { filename: file.name, mime_type: "image/jpeg", content: canvas.toDataURL("image/jpeg", 0.85).split(",")[1] }
  } finally { URL.revokeObjectURL(url) }
}

export default function DevicePhotosInput({ value, onChange, disabled, uploading, onUploadingChange }: {
  value: DevicePhoto[]; onChange: (value: DevicePhoto[]) => void
  disabled?: boolean; uploading: boolean; onUploadingChange: (value: boolean) => void
}) {
  const { t } = useDeviceTranslations()
  const id = useId()
  async function upload(files: File[]) {
    if (!files.length || uploading || disabled) return
    if (value.length + files.length > 8 || files.some((file) => !["image/jpeg", "image/png", "image/webp"].includes(file.type))) {
      notifyError(t("deviceScan.photosHelp")); return
    }
    onUploadingChange(true)
    try {
      const result = await sdk.client.fetch<{ photos: DevicePhoto[] }>("/admin/devices/photos", {
        method: "POST", body: { files: await Promise.all(files.map(photoContent)) },
      })
      onChange([...value, ...result.photos])
    } catch { notifyError(t("deviceScan.photosError")) }
    finally { onUploadingChange(false) }
  }
  return <section className="space-y-2 rounded-lg border p-3">
    <Text size="small" weight="plus">{t("deviceScan.photos")}</Text>
    <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.photosHelp")}</Text>
    <div className="flex flex-wrap gap-3">
      {value.map((photo, index) => <div key={photo.id} className="space-y-1">
        <a href={photo.url} target="_blank" rel="noreferrer"><img src={photo.url} alt={`${t("deviceScan.photos")} ${index + 1}`} style={{ width: 96, height: 96, objectFit: "cover", borderRadius: 8 }} /></a>
        <Button size="small" variant="secondary" disabled={disabled || uploading} onClick={() => onChange(value.filter((entry) => entry.id !== photo.id))}>{t("deviceScan.photoRemove")}</Button>
      </div>)}
    </div>
    <label className="block space-y-1" htmlFor={id}><Text size="small">{uploading ? t("deviceScan.photosUploading") : t("deviceScan.photosAdd")}</Text>
      <input id={id} type="file" accept="image/jpeg,image/png,image/webp" multiple disabled={disabled || uploading || value.length >= 8} onChange={(event) => { void upload(Array.from(event.target.files || [])); event.target.value = "" }} />
    </label>
    <label className="block space-y-1"><Text size="small">{t("deviceScan.photosCamera")}</Text>
      <input type="file" accept="image/jpeg,image/png,image/webp" capture="environment" disabled={disabled || uploading || value.length >= 8} onChange={(event) => { void upload(Array.from(event.target.files || [])); event.target.value = "" }} />
    </label>
  </section>
}
