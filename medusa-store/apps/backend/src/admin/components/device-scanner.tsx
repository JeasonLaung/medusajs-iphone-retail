import { notifyError } from "../lib/notifications"
import { ClearableInput } from "./clearable-input"
import { useTranslation } from "react-i18next"
import { BrowserMultiFormatReader, type IScannerControls } from "@zxing/browser"
import { Button, Text } from "@medusajs/ui"
import { useEffect, useRef, useState } from "react"
import { classifyDeviceScans } from "../lib/device-scan-values"

export default function DeviceScanner({
  onScan,
  onScanMany,
  recognizeSku = false,
  showManualInput = true,
  autoStart = false,
}: {
  onScan: (value: string) => void
  onScanMany?: (values: string[], skuSearch?: string) => void
  recognizeSku?: boolean
  showManualInput?: boolean
  autoStart?: boolean
}) {
  const { t } = useTranslation()
  const video = useRef<HTMLVideoElement>(null)
  const controls = useRef<IScannerControls | null>(null)
  const generation = useRef(0)
  const callback = useRef(onScan)
  callback.current = onScan
  const batchCallback = useRef(onScanMany)
  batchCallback.current = onScanMany
  const frameTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const detected = useRef<string[]>([])
  const latestFrame = useRef<ImageData | null>(null)
  const ocrAbort = useRef<AbortController | null>(null)
  const [found, setFound] = useState<string[]>([])
  const [running, setRunning] = useState(false)
  const [value, setValue] = useState("")
  const [last, setLast] = useState("")
  const [imageBusy, setImageBusy] = useState(false)
  const [ocrBusy, setOcrBusy] = useState(false)
  const [ocrNote, setOcrNote] = useState("")
  function stop() {
    generation.current += 1
    ocrAbort.current?.abort()
    ocrAbort.current = null
    controls.current?.stop()
    controls.current = null
    clearTimeout(frameTimer.current)
    const stream = video.current?.srcObject
    if (typeof MediaStream !== "undefined" && stream instanceof MediaStream)
      stream.getTracks().forEach((track) => track.stop())
    setRunning(false)
  }
  useEffect(() => {
    if (autoStart) void start()
    return () => {
      generation.current += 1
      ocrAbort.current?.abort()
      clearTimeout(frameTimer.current)
      controls.current?.stop()
      const stream = video.current?.srcObject
      if (typeof MediaStream !== "undefined" && stream instanceof MediaStream)
        stream.getTracks().forEach((track) => track.stop())
    }
  }, [autoStart])
  async function acceptMany(values: string[], image?: Blob | ImageData) {
    stop()
    const current = generation.current
    let skuSearch = ""
    if (recognizeSku && image) {
      const controller = new AbortController()
      ocrAbort.current = controller
      setOcrBusy(true)
      setOcrNote("")
      try {
        const { recognizeDeviceLabel } = await import("../lib/device-label-ocr")
        skuSearch = await recognizeDeviceLabel(image, controller.signal)
        if (generation.current === current && !skuSearch) setOcrNote(t("deviceScan.labelTextNotFound"))
      } catch {
        if (generation.current === current) notifyError(t("deviceScan.labelTextError"))
      } finally {
        if (generation.current === current) {
          setOcrBusy(false)
          ocrAbort.current = null
        }
      }
    }
    if (generation.current !== current) return
    if (!values.length && !skuSearch) {
      notifyError(t("deviceScan.imageError"))
      return
    }
    setLast(values.join(" / "))
    setValue("")
    batchCallback.current?.(values, skuSearch)
    detected.current = []
    latestFrame.current = null
    setFound([])
  }
  async function startMultiple(current: number) {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: false, video: { facingMode: { ideal: "environment" }, width: { ideal: 1920 }, height: { ideal: 1080 } },
    })
    if (generation.current !== current || !video.current) {
      stream.getTracks().forEach((track) => track.stop())
      return
    }
    video.current.srcObject = stream
    await video.current.play()
    const { decodeDeviceBarcodes } = await import("../lib/device-multi-barcode")
    const canvas = document.createElement("canvas")
    const scanFrame = async () => {
      if (generation.current !== current || !video.current) return
      try {
        const preview = video.current
        if (preview.readyState >= 2 && preview.videoWidth) {
          const scale = Math.min(1, 1920 / preview.videoWidth)
          canvas.width = Math.round(preview.videoWidth * scale)
          canvas.height = Math.round(preview.videoHeight * scale)
          const context = canvas.getContext("2d", { willReadFrequently: true })!
          context.drawImage(preview, 0, 0, canvas.width, canvas.height)
          latestFrame.current = context.getImageData(0, 0, canvas.width, canvas.height)
          const values = await decodeDeviceBarcodes(latestFrame.current)
          if (generation.current !== current) return
          detected.current = [...new Set([...detected.current, ...values])]
          setFound(detected.current)
          const parsed = classifyDeviceScans(detected.current)
          if (parsed.serials.length === 1 && parsed.radios.length === 2 && !parsed.unknown.length) {
            await acceptMany(detected.current, latestFrame.current)
            return
          }
        }
        if (generation.current === current) frameTimer.current = setTimeout(() => void scanFrame(), 300)
      } catch {
        if (generation.current === current) {
          stop()
          notifyError(t("deviceScan.imageError"))
        }
      }
    }
    await scanFrame()
  }
  async function start() {
    stop()
    detected.current = []
    latestFrame.current = null
    setFound([])
    setOcrNote("")
    if (!window.isSecureContext) {
      notifyError(t("deviceScan.cameraHttps"))
      return
    }
    if (!navigator.mediaDevices?.getUserMedia) {
      notifyError(t("deviceScan.cameraUnsupported"))
      return
    }
    const current = generation.current
    setRunning(true)
    let accepted = false
    try {
      if (batchCallback.current) {
        await startMultiple(current)
        return
      }
      const reader = new BrowserMultiFormatReader()
      const scanner = await reader.decodeFromConstraints(
        { audio: false, video: { facingMode: { ideal: "environment" } } },
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
        const name = (e as Error).name
        const key = name === "NotAllowedError" || name === "SecurityError"
          ? "cameraPermission"
          : name === "NotFoundError"
            ? "cameraNotFound"
            : name === "NotReadableError"
              ? "cameraBusy"
              : "cameraError"
        notifyError(t(`deviceScan.${key}`, { message: (e as Error).message }))
      }
    }
  }
  async function decodeImage(file?: File) {
    if (!file) return
    stop()
    const current = generation.current
    setImageBusy(true)
    setOcrNote("")
    const url = URL.createObjectURL(file)
    try {
      if (batchCallback.current) {
        const { decodeDeviceBarcodes } = await import("../lib/device-multi-barcode")
        const values = await decodeDeviceBarcodes(file)
        if (generation.current !== current) return
        if (!values.length && !recognizeSku) throw new Error("No barcodes")
        await acceptMany(values, file)
        return
      }
      const result = await new BrowserMultiFormatReader().decodeFromImageUrl(
        url,
      )
      const text = result.getText().trim()
      setLast(text)
      setValue(text)
      callback.current(text)
    } catch {
      if (generation.current === current) notifyError(t("deviceScan.imageError"))
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
      <p className="text-sm text-ui-fg-subtle">{t(onScanMany ? "deviceScan.scannerMultiHelp" : "deviceScan.scannerHelp")}</p>
      {recognizeSku && onScanMany && <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.labelTextHelp")}</Text>}
      {ocrBusy && <Text size="small" role="status">{t("deviceScan.labelTextReading")}</Text>}
      {ocrNote && <Text size="small" role="status" className="text-ui-fg-subtle">{ocrNote}</Text>}
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
          disabled={imageBusy || ocrBusy}
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
            disabled={imageBusy || ocrBusy}
            onChange={(e) => {
              void decodeImage(e.target.files?.[0])
              e.target.value = ""
            }}
          />
        </label>
      </div>
      {onScanMany && found.length > 0 && (
        <div className="space-y-2">
          <p className="break-all text-sm" role="status">{t("deviceScan.scannerFound", { count: found.length })}：{found.join(" / ")}</p>
          <Button type="button" variant="secondary" disabled={imageBusy || ocrBusy} onClick={() => void acceptMany(found, latestFrame.current || undefined)}>{t("deviceScan.scannerUseFound")}</Button>
        </div>
      )}
      {showManualInput && (
        <form
          className="flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            stop()
            if (value.trim()) {
              if (batchCallback.current) void acceptMany([value.trim()])
              else {
                callback.current(value.trim())
                setLast(value.trim())
              }
            }
          }}
        >
          <ClearableInput
            aria-label={t("deviceScan.manualInput")}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="min-w-0 flex-1 rounded-md border bg-ui-bg-base px-3 py-2"
            placeholder={t("deviceScan.manualPlaceholder")}
            autoComplete="off"
          />
          <Button type="submit" variant="secondary" disabled={!value.trim() || imageBusy || ocrBusy}>
            {t("deviceScan.useIdentifier")}
          </Button>
        </form>
      )}
      {last && (
        <p className="break-all text-sm" role="status">
          {t("deviceScan.readPrefix")}
          {last}
        </p>
      )}
    </div>
  )
}
