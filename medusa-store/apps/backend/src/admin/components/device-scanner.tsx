import { useTranslation } from "react-i18next"
import { BrowserMultiFormatReader, type IScannerControls } from "@zxing/browser"
import { Button } from "@medusajs/ui"
import { useEffect, useRef, useState } from "react"

export default function DeviceScanner({
  onScan,
}: {
  onScan: (value: string) => void
}) {
  const { t } = useTranslation()
  const video = useRef<HTMLVideoElement>(null)
  const controls = useRef<IScannerControls | null>(null)
  const generation = useRef(0)
  const callback = useRef(onScan)
  callback.current = onScan
  const [running, setRunning] = useState(false)
  const [error, setError] = useState("")
  const [value, setValue] = useState("")
  const [last, setLast] = useState("")
  const [imageBusy, setImageBusy] = useState(false)
  function stop() {
    generation.current += 1
    controls.current?.stop()
    controls.current = null
    const stream = video.current?.srcObject
    if (stream instanceof MediaStream)
      stream.getTracks().forEach((track) => track.stop())
    setRunning(false)
  }
  useEffect(
    () => () => {
      generation.current += 1
      controls.current?.stop()
      const stream = video.current?.srcObject
      if (stream instanceof MediaStream)
        stream.getTracks().forEach((track) => track.stop())
    },
    [],
  )
  async function start() {
    stop()
    setError("")
    if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
      setError(t("deviceScan.cameraHttps"))
      return
    }
    const current = generation.current
    setRunning(true)
    let accepted = false
    try {
      const reader = new BrowserMultiFormatReader()
      const scanner = await reader.decodeFromVideoDevice(
        undefined,
        video.current!,
        (result, _error, scanner) => {
          if (!result || accepted || generation.current !== current) return
          accepted = true
          const text = result.getText().trim()
          scanner.stop()
          controls.current = null
          setRunning(false)
          setLast(text)
          setValue(text)
          callback.current(text)
        },
      )
      if (accepted || generation.current !== current) scanner.stop()
      else controls.current = scanner
    } catch (e) {
      if (generation.current === current) {
        stop()
        setError(t("deviceScan.cameraError", { message: (e as Error).message }))
      }
    }
  }
  async function decodeImage(file?: File) {
    if (!file) return
    stop()
    setError("")
    setImageBusy(true)
    const url = URL.createObjectURL(file)
    try {
      const result = await new BrowserMultiFormatReader().decodeFromImageUrl(
        url,
      )
      const text = result.getText().trim()
      setLast(text)
      setValue(text)
      callback.current(text)
    } catch {
      setError(t("deviceScan.imageError"))
    } finally {
      URL.revokeObjectURL(url)
      setImageBusy(false)
    }
  }
  return (
    <div
      className="space-y-3 rounded-lg border p-4"
      data-testid="device-scanner"
    >
      <p className="font-medium">{t("deviceScan.scannerTitle")}</p>
      <p className="text-sm text-ui-fg-subtle">{t("deviceScan.scannerHelp")}</p>
      <video
        ref={video}
        muted
        playsInline
        className={`w-full rounded-lg bg-black ${running ? "block" : "hidden"}`}
        style={{ maxHeight: 300 }}
        aria-label={t("deviceScan.cameraPreview")}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          onClick={running ? stop : start}
          type="button"
        >
          {running ? t("deviceScan.stopCamera") : t("deviceScan.startCamera")}
        </Button>
        <label className="cursor-pointer rounded-md border px-3 py-2 text-sm">
          {imageBusy ? t("deviceScan.decoding") : t("deviceScan.uploadPhoto")}
          <input
            aria-label={t("deviceScan.uploadLabel")}
            type="file"
            accept="image/*"
            capture="environment"
            className="hidden"
            disabled={imageBusy}
            onChange={(e) => {
              void decodeImage(e.target.files?.[0])
              e.target.value = ""
            }}
          />
        </label>
      </div>
      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          stop()
          if (value.trim()) {
            callback.current(value.trim())
            setLast(value.trim())
          }
        }}
      >
        <input
          aria-label={t("deviceScan.manualInput")}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="min-w-0 flex-1 rounded-md border bg-ui-bg-base px-3 py-2"
          placeholder={t("deviceScan.manualPlaceholder")}
          autoComplete="off"
        />
        <Button type="submit" variant="secondary" disabled={!value.trim()}>
          {t("deviceScan.useIdentifier")}
        </Button>
      </form>
      {last && (
        <p className="break-all text-sm" role="status">
          {t("deviceScan.readPrefix")}
          {last}
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-ui-fg-error">
          {error}
        </p>
      )}
    </div>
  )
}
