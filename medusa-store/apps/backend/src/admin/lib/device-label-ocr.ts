import { createWorker, OEM, PSM, type Worker } from "tesseract.js"
import { simd } from "wasm-feature-detect"
import workerUrl from "tesseract.js/dist/worker.min.js?url"
import coreUrl from "tesseract.js-core/tesseract-core-lstm.wasm.js?url"
import simdCoreUrl from "tesseract.js-core/tesseract-core-simd-lstm.wasm.js?url"
import englishUrl from "@tesseract.js-data/eng/4.0.0_best_int/eng.traineddata.gz?url"
import { deviceLabelSearch } from "./device-label-search"

// Every asset is bundled with the admin. Photos stay in the browser.
export async function recognizeDeviceLabel(image: Blob | ImageData, signal: AbortSignal): Promise<string> {
  let worker: Worker | undefined
  let bootstrapUrl: string | undefined
  let cancelled = false
  let timeout: ReturnType<typeof setTimeout> | undefined
  let rejectCancelled: (reason: Error) => void = () => undefined
  const cancellation = new Promise<never>((_, reject) => { rejectCancelled = reject })
  const cancel = () => {
    cancelled = true
    void worker?.terminate()
    rejectCancelled(new Error("Label recognition cancelled or timed out"))
  }
  signal.addEventListener("abort", cancel, { once: true })
  const work = async () => {
    if (signal.aborted) throw new Error("Label recognition cancelled")
    const languageUrl = new URL(englishUrl, location.href).href
    // The OCR loader expects a stable eng.traineddata.gz filename, while Vite
    // hashes assets in production. Map that one request to the bundled asset.
    bootstrapUrl = URL.createObjectURL(new Blob([`
      // A worker that fails during initialization has no public handle yet.
      // Bound its lifetime and close it immediately on an engine error.
      setTimeout(() => self.close(), 50000)
      const sendResult = self.postMessage.bind(self)
      self.postMessage = (message, ...args) => {
        sendResult(message, ...args)
        if (message.status === "reject") self.close()
      }
      const localFetch = self.fetch.bind(self)
      self.fetch = (resource, init) => localFetch(
        typeof resource === "string" && resource.endsWith("/eng.traineddata.gz")
          ? ${JSON.stringify(languageUrl)} : resource, init)
      importScripts(${JSON.stringify(new URL(workerUrl, location.href).href)})
    `], { type: "text/javascript" }))
    worker = await createWorker("eng", OEM.LSTM_ONLY, {
      workerPath: bootstrapUrl,
      workerBlobURL: false,
      corePath: new URL(await simd() ? simdCoreUrl : coreUrl, location.href).href,
      langPath: new URL(".", languageUrl).href,
      cacheMethod: "none",
      errorHandler: (error) => rejectCancelled(new Error(String(error))),
    })
    if (cancelled) { await worker.terminate(); throw new Error("Label recognition cancelled") }
    await worker.setParameters({ tessedit_pageseg_mode: PSM.SPARSE_TEXT })
    const bitmap = image instanceof Blob ? await createImageBitmap(image) : null
    const source = document.createElement("canvas")
    source.width = bitmap?.width || (image as ImageData).width
    source.height = bitmap?.height || (image as ImageData).height
    const sourceContext = source.getContext("2d")!
    if (bitmap) { sourceContext.drawImage(bitmap, 0, 0); bitmap.close() }
    else sourceContext.putImageData(image as ImageData, 0, 0)
    try {
      for (const angle of [0, 90, 270, 180]) {
        if (cancelled) throw new Error("Label recognition cancelled")
        const canvas = document.createElement("canvas")
        const scale = Math.min(1, 2200 / Math.max(source.width, source.height))
        const width = Math.round(source.width * scale)
        const height = Math.round(source.height * scale)
        canvas.width = angle % 180 ? height : width
        canvas.height = angle % 180 ? width : height
        const context = canvas.getContext("2d")!
        context.fillStyle = "white"
        context.fillRect(0, 0, canvas.width, canvas.height)
        context.translate(canvas.width / 2, canvas.height / 2)
        context.rotate(angle * Math.PI / 180)
        context.drawImage(source, -width / 2, -height / 2, width, height)
        const { data } = await worker.recognize(canvas)
        const search = deviceLabelSearch(data.text)
        if (search) return search
      }
      return ""
    } finally { await worker.terminate(); worker = undefined }
  }
  try {
    timeout = setTimeout(cancel, 45000)
    return await Promise.race([work(), cancellation])
  } finally {
    clearTimeout(timeout)
    signal.removeEventListener("abort", cancel)
    if (worker) await worker.terminate()
    if (bootstrapUrl) URL.revokeObjectURL(bootstrapUrl)
  }
}
