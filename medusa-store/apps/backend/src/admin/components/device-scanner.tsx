import { BrowserMultiFormatReader, type IScannerControls } from "@zxing/browser"
import { Button } from "@medusajs/ui"
import { useEffect, useRef, useState } from "react"

export default function DeviceScanner({
  onScan,
}: {
  onScan: (value: string) => void
}) {
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
      setError(
        "手机相机需要通过 HTTPS 打开后台，并允许相机权限。也可以拍照上传识别或手动输入。",
      )
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
        setError(
          `无法打开相机：${(e as Error).message}。请检查权限，或拍照上传 / 手动输入。`,
        )
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
      setError(
        "未识别到条形码或二维码，请拍摄清晰完整的标签。普通文字请手动输入。",
      )
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
      <p className="font-medium">扫码读取序列号 / IMEI</p>
      <p className="text-sm text-ui-fg-subtle">
        支持标签上的条形码、二维码。扫码只填写字段，核对后提交操作。
      </p>
      <video
        ref={video}
        muted
        playsInline
        className={`w-full rounded-lg bg-black ${running ? "block" : "hidden"}`}
        style={{ maxHeight: 300 }}
        aria-label="扫码相机预览"
      />
      <div className="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          onClick={running ? stop : start}
          type="button"
        >
          {running ? "关闭相机" : "打开手机相机"}
        </Button>
        <label className="cursor-pointer rounded-md border px-3 py-2 text-sm">
          {imageBusy ? "识别中…" : "拍照 / 上传标签"}
          <input
            aria-label="上传标签图片"
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
          aria-label="手动输入序列号或 IMEI"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="min-w-0 flex-1 rounded-md border bg-ui-bg-base px-3 py-2"
          placeholder="手动输入或使用扫码枪"
          autoComplete="off"
        />
        <Button type="submit" variant="secondary" disabled={!value.trim()}>
          使用号码
        </Button>
      </form>
      {last && (
        <p className="break-all text-sm" role="status">
          已读取：{last}
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
