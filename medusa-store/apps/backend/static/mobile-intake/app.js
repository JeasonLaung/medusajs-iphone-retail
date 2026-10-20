/* global Vue, vant, ZXingWASM, intakeLocales */
const intakeToken = location.hash.slice(1)
Vue.createApp({
  setup() {
    const { ref, reactive, computed, nextTick, onMounted, watch } = Vue
    const lang = ref(new URLSearchParams(location.search).get("lang") === "en" ? "en" : "zh")
    const t = (key) => intakeLocales[lang.value][key] || key
    const session = ref(null), loading = ref(true), invalid = ref(false), busy = ref(false)
    const showVariants = ref(false), variants = ref([]), variant = ref(null), query = ref("")
    const searching = ref(false), hasMore = ref(false), showScanner = ref(false)
    const showStats = ref(false), intakeRecords = ref([]), deletingId = ref("")
    const modelStats = computed(() => {
      const models = new Map()
      for (const item of intakeRecords.value) {
        models.set(item.model, (models.get(item.model) || 0) + 1)
      }
      return Array.from(models, ([model, count]) => ({ model, count })).sort((a, b) => b.count - a.count || a.model.localeCompare(b.model))
    })
    const photos = ref([]), uploading = ref(false)
    const cameraLoading = ref(false), scanTarget = ref(""), scanStatus = ref("")
    const scanImageInput = ref(null)
    let scannerLibraryPromise
    const form = reactive({ serial_number: "", imei: "", imei2: "", eid: "", cost: "", bin: "", notes: "" })
    let offset = 0, controls = null, scanGeneration = 0, expiryTimer = null, searchGeneration = 0
    let scanTimer = null, variantSearchTimer = null
    const seen = new Set()
    const validRadio = (value) => !value || /^\d{15}$/.test(value.trim())
    const validEid = (value) => !value || /^\d{32}$/.test(value.trim())
    const validCost = (value) => value === "" || (/^\d+(?:\.\d{1,2})?$/.test(value) && Number(value) <= 1000000)
    const canSubmit = computed(() => Boolean(session.value && !loading.value && !invalid.value && !busy.value && !uploading.value && !deletingId.value && variant.value && form.serial_number.trim() && form.serial_number.trim().length <= 100 && validRadio(form.imei) && validRadio(form.imei2) && validEid(form.eid) && validCost(form.cost) && (!form.imei.trim() || form.imei.trim() !== form.imei2.trim()) && (!variant.value.requires_imei || form.imei.trim() || form.imei2.trim() || form.eid.trim())))
    const errorToast = (message) => vant.showToast({ message, position: "top", duration: 4000 })
    function localizeError(message) {
      if (message.includes("MOBILE_INTAKE_DELETE_DENIED")) return t("deleteDenied")
      const match = Object.entries(intakeLocales.zh.errors).find(([, value]) => message.includes(value))
      return match ? intakeLocales[lang.value].errors[match[0]] : t("requestError")
    }
    async function api(path, options = {}) {
      const response = await fetch(`/mobile-intake${path}`, {
        ...options, credentials: "omit", cache: "no-store",
        headers: { "Content-Type": "application/json", "x-intake-token": intakeToken },
        signal: AbortSignal.timeout(30000),
      })
      const data = await response.json()
      if (!response.ok) {
        if ([401, 403, 404].includes(response.status)) { invalid.value = true; closeScanner() }
        throw new Error(data.message || "")
      }
      return data
    }
    async function loadSession() {
      loading.value = true
      invalid.value = false
      try {
        if (!intakeToken) throw new Error("missing token")
        session.value = await api("")
        clearTimeout(expiryTimer)
        expiryTimer = setTimeout(() => { invalid.value = true; closeScanner() }, Math.max(0, session.value.expires_at - Date.now()))
      } catch { invalid.value = true }
      finally { loading.value = false }
    }
    watch(query, () => {
      clearTimeout(variantSearchTimer)
      ++searchGeneration
      if (!showVariants.value) return
      offset = 0
      variants.value = []
      hasMore.value = false
      searching.value = true
      variantSearchTimer = setTimeout(() => { void searchVariants(true) }, 300)
    }, { flush: "sync" })
    watch(showVariants, (open) => {
      if (!open) {
        clearTimeout(variantSearchTimer)
        ++searchGeneration
        searching.value = false
      }
    }, { flush: "sync" })
    async function searchVariants(reset) {
      clearTimeout(variantSearchTimer)
      const generation = ++searchGeneration
      searching.value = true
      if (reset) { offset = 0; variants.value = []; hasMore.value = false }
      try {
        const result = await api(`/variants?q=${encodeURIComponent(query.value.trim())}&offset=${offset}`)
        if (generation !== searchGeneration) return
        variants.value = reset ? result.variants : [...variants.value, ...result.variants]
        offset += 20
        hasMore.value = offset < result.count
      } catch (error) { if (generation === searchGeneration) errorToast(localizeError(error.message)) }
      finally { if (generation === searchGeneration) searching.value = false }
    }
    async function submit() {
      if (!canSubmit.value) return
      if (!variant.value) return errorToast(t("chooseSku"))
      if (variant.value.requires_imei && !form.imei.trim() && !form.imei2.trim() && !form.eid.trim()) return errorToast(t("imeiRequired"))
      if (form.imei.trim() && form.imei.trim() === form.imei2.trim()) return errorToast(t("duplicateRadio"))
      if (!validCost(form.cost)) return errorToast(t("invalidCost"))
      busy.value = true
      try {
        const receipt = await api("", { method: "POST", body: JSON.stringify({
          variant_id: variant.value.id, serial_number: form.serial_number.trim().toUpperCase(),
          ...(form.imei.trim() ? { imei: form.imei.trim() } : {}),
          ...(form.imei2.trim() ? { imei2: form.imei2.trim() } : {}),
          ...(form.eid.trim() ? { eid: form.eid.trim() } : {}),
          // Device acquisition cost uses the existing device ledger's integer cents.
          cost_cents: Math.round(Number(form.cost || 0) * 100), currency_code: "eur",
          bin: form.bin.trim(), notes: form.notes.trim(), photos: photos.value,
        }) })
        intakeRecords.value.unshift({
          id: receipt.result.device_ids[0],
          serial: form.serial_number.trim().toUpperCase(), imei: form.imei.trim(), imei2: form.imei2.trim(), eid: form.eid.trim(),
          title: variant.value.title, model: variant.value.title.split(" · ")[0] || variant.value.title,
          sku: variant.value.sku,
        })
        form.serial_number = ""; form.imei = ""; form.imei2 = ""; form.eid = ""; form.notes = ""
        photos.value = []
        seen.clear()
        vant.showToast({ message: t(session.value.category === "sale" ? "success" : "reviewSuccess"), position: "top" })
      } catch (error) { errorToast(localizeError(error.message)) }
      finally { busy.value = false }
    }
    async function deleteIntake(item) {
      if (busy.value || uploading.value || deletingId.value || invalid.value) return
      deletingId.value = item.id
      try {
        try {
          await vant.showConfirmDialog({ title: t("deleteDevice"), message: t("deleteConfirm").replace("{{serial}}", item.serial), confirmButtonText: t("deleteDevice"), cancelButtonText: t("cancel") })
        } catch { return }
        await api(`/devices/${encodeURIComponent(item.id)}`, { method: "DELETE" })
        intakeRecords.value = intakeRecords.value.filter((record) => record.id !== item.id)
        vant.showToast({ message: t("deleteSuccess"), position: "top" })
      } catch (error) { errorToast(localizeError(error.message)) }
      finally { deletingId.value = "" }
    }
    const photoPreviews = computed(() => photos.value.map((photo) => ({ ...photo, isImage: true })))
    function removePhoto(_photo, { index }) {
      if (!busy.value && !uploading.value) photos.value.splice(index, 1)
    }
    async function uploadPhotos(items) {
      const files = (Array.isArray(items) ? items : [items]).map((item) => item.file).filter(Boolean)
      if (!files.length || busy.value || uploading.value) return
      if (photos.value.length + files.length > 8 || files.some((file) => !["image/jpeg", "image/png", "image/webp"].includes(file.type))) return errorToast(t("photoHelp"))
      uploading.value = true
      try {
        const encoded = await Promise.all(files.map(async (file) => {
          const url = URL.createObjectURL(file)
          try {
            const image = new Image(); image.src = url; await image.decode()
            const scale = Math.min(1, 1800 / Math.max(image.width, image.height))
            const canvas = document.createElement("canvas")
            canvas.width = Math.round(image.width * scale); canvas.height = Math.round(image.height * scale)
            const context = canvas.getContext("2d"); context.fillStyle = "#fff"; context.fillRect(0, 0, canvas.width, canvas.height)
            context.drawImage(image, 0, 0, canvas.width, canvas.height)
            return { filename: file.name, mime_type: "image/jpeg", content: canvas.toDataURL("image/jpeg", 0.85).split(",")[1] }
          } finally { URL.revokeObjectURL(url) }
        }))
        const result = await api("/photos", { method: "POST", body: JSON.stringify({ files: encoded }) })
        photos.value.push(...result.photos)
      } catch { errorToast(t("photoError")) }
      finally { uploading.value = false }
    }
    function scanValue(raw) {
      if (seen.has(raw)) return false
      let accepted = false
      for (const part of raw.split(/[\r\n;,|]+/)) {
        const value = part.trim().toUpperCase().replace(/^(?:S\/?N|SERIAL(?:\s*NUMBER)?|IMEI\s*[12]?|EID\s*[12]?|序列号)\s*[:：=]\s*/, "")
        if (!value) continue
        if (/^(?:\d{15}|\d{32})$/.test(value)) {
          if (scanTarget.value === "serial_number") continue
          if (scanTarget.value) {
            if ((scanTarget.value === "eid") !== (value.length === 32)) continue
            form[scanTarget.value] = value
            accepted = true
            break
          }
          if (value.length === 32) {
            if (!form.eid) { form.eid = value; accepted = true }
            else if (form.eid !== value) errorToast(t("scanConflict"))
            continue
          }
          if ([form.imei, form.imei2].includes(value)) continue
          if (!form.imei) form.imei = value
          else if (!form.imei2) form.imei2 = value
          else errorToast(t("scanConflict"))
          accepted = true
        } else if (/^[A-Z0-9-]{1,100}$/.test(value) && (!/^\d+$/.test(value) || value.length < 15)) {
          if (scanTarget.value && scanTarget.value !== "serial_number") continue
          if (scanTarget.value === "serial_number") {
            form.serial_number = value
            accepted = true
            break
          }
          if (!form.serial_number) form.serial_number = value
          else if (form.serial_number.toUpperCase() !== value) errorToast(t("scanConflict"))
          accepted = true
        } else errorToast(t("scanUnknown"))
      }
      if (accepted) {
        seen.add(raw)
        scanStatus.value = t("scanDetected")
        if (scanTarget.value) closeScanner()
      }
      return accepted
    }
    function stopScanner() {
      scanGeneration++
      cameraLoading.value = false
      clearTimeout(scanTimer)
      if (controls) { controls.stop(); controls = null }
      const video = document.getElementById("scan-video")
      if (video?.srcObject) { video.srcObject.getTracks().forEach((track) => track.stop()); video.srcObject = null }
    }
    function closeScanner() { stopScanner(); showScanner.value = false }
    function loadScannerLibrary() {
      if (window.ZXingWASM) return Promise.resolve()
      if (!scannerLibraryPromise) {
        scannerLibraryPromise = new Promise((resolve, reject) => {
          const script = document.createElement("script")
          script.src = "./vendor/zxing-wasm-reader-3.1.4.js"
          const timer = setTimeout(() => script.onerror(), 20000)
          script.onload = async () => {
            try {
              await ZXingWASM.prepareZXingModule({
                overrides: { locateFile: (name, prefix) => name.endsWith(".wasm")
                  ? new URL("./vendor/zxing_reader-3.1.4.wasm", location.href).href : prefix + name },
                fireImmediately: true,
              })
              clearTimeout(timer)
              resolve()
            } catch { script.onerror() }
          }
          script.onerror = () => {
            clearTimeout(timer)
            script.remove()
            scannerLibraryPromise = undefined
            reject(new Error("Scanner library unavailable"))
          }
          document.head.appendChild(script)
        })
      }
      return scannerLibraryPromise
    }
    function chooseScanImage() {
      closeScanner()
      scanImageInput.value?.click()
    }
    async function scanImage(event) {
      const file = event.target.files?.[0]
      event.target.value = ""
      if (!file) return
      try {
        await loadScannerLibrary()
        const results = await ZXingWASM.readBarcodes(file, { tryHarder: true, maxNumberOfSymbols: 12 })
        seen.clear()
        if (!results.length) return errorToast(t("scanNoResult"))
        for (const result of results) scanValue(result.text)
      } catch { errorToast(t("photoError")) }
    }
    async function openScanner(target = "") {
      stopScanner()
      scanTarget.value = typeof target === "string" ? target : ""
      scanStatus.value = t("scanWaiting")
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) return chooseScanImage()
      try {
        const devices = await navigator.mediaDevices.enumerateDevices()
        if (!devices.some((device) => device.kind === "videoinput")) return chooseScanImage()
      } catch { /* Request camera access below when enumeration is unavailable. */ }
      seen.clear()
      showScanner.value = true
      cameraLoading.value = true
      await nextTick()
      const generation = ++scanGeneration
      try {
        await loadScannerLibrary()
        if (generation !== scanGeneration || !showScanner.value) return
        const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: "environment" }, width: { ideal: 1920 }, height: { ideal: 1080 } }, audio: false })
        if (generation !== scanGeneration || !showScanner.value) { stream.getTracks().forEach((track) => track.stop()); return }
        const video = document.getElementById("scan-video")
        video.srcObject = stream
        await video.play()
        const track = stream.getVideoTracks()[0]
        const capabilities = track.getCapabilities?.()
        if (capabilities?.focusMode?.includes("continuous")) void track.applyConstraints({ advanced: [{ focusMode: "continuous" }] }).catch(() => {})
        const canvas = document.createElement("canvas")
        const context = canvas.getContext("2d", { willReadFrequently: true })
        let attempts = 0
        const frame = async () => {
          if (generation !== scanGeneration || !showScanner.value) return
          try {
            if (video.readyState >= 2 && video.videoWidth) {
              const scale = Math.min(1, 1920 / video.videoWidth)
              canvas.width = Math.round(video.videoWidth * scale)
              canvas.height = Math.round(video.videoHeight * scale)
              context.drawImage(video, 0, 0, canvas.width, canvas.height)
              const results = await ZXingWASM.readBarcodes(context.getImageData(0, 0, canvas.width, canvas.height), { tryHarder: true, maxNumberOfSymbols: 12 })
              if (generation !== scanGeneration || !showScanner.value) return
              for (const result of results) {
                scanValue(result.text)
                if (generation !== scanGeneration) return
              }
              if (++attempts === 20 && !seen.size) scanStatus.value = t("scanNoResult")
            }
            if (generation === scanGeneration) scanTimer = setTimeout(frame, 250)
          } catch { if (generation === scanGeneration) { closeScanner(); errorToast(t("cameraError")) } }
        }
        cameraLoading.value = false
        await frame()
      } catch { if (generation === scanGeneration) chooseScanImage() }
      finally { if (generation === scanGeneration) cameraLoading.value = false }
    }
    function toggleLanguage() {
      lang.value = lang.value === "zh" ? "en" : "zh"
      document.documentElement.lang = lang.value === "zh" ? "zh-CN" : "en"
      document.title = t("title")
      const url = new URL(location.href); url.searchParams.set("lang", lang.value); window.history.replaceState(null, "", url)
    }
    onMounted(() => {
      document.documentElement.lang = lang.value === "zh" ? "zh-CN" : "en"
      document.title = t("title")
      loadSession()
      document.addEventListener("visibilitychange", () => { if (document.hidden) closeScanner() })
      window.addEventListener("pagehide", () => { clearTimeout(variantSearchTimer); ++searchGeneration; stopScanner() })
    })
    return { canSubmit, deletingId, deleteIntake, scanImageInput, scanImage, chooseScanImage, showStats, intakeRecords, modelStats, cameraLoading, scanTarget, scanStatus, photos, photoPreviews, removePhoto, uploading, uploadPhotos, lang, t, session, loading, invalid, busy, showVariants, variants, variant, query,
      searching, hasMore, showScanner, form, validRadio, validEid, validCost, loadSession,
      searchVariants, submit, openScanner, closeScanner, stopScanner, toggleLanguage }
  },
}).use(vant).mount("#app")
