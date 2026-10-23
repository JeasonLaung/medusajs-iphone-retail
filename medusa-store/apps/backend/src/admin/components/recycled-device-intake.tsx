import { Button, FocusModal, Heading, Input, Label, Text, Textarea, toast, usePrompt } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { sdk } from "../lib/sdk"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { notifyError } from "../lib/notifications"
import { localizeDeviceMessage } from "../lib/device-messages"
import { isImei } from "../../lib/device-identity"
import { assessmentNotes, completeAssessment, eligible, emptyAssessment, faultIds, glassIds, gradeIds, skipScreen, validBattery, validMulti, type RecycledAssessment } from "../lib/recycled-assessment"
import RecycledAssessmentSummary from "./recycled-assessment-summary"
import DevicePhotosInput, { type DevicePhoto } from "./device-photos-input"

const selectClass = "w-full min-w-0 rounded-md border border-ui-border-base bg-ui-bg-field px-3 py-2 text-sm outline-none focus:border-ui-border-interactive focus:shadow-borders-interactive-with-focus"
const normalize = (value: string) => value.replace(/\s+/g, "").toUpperCase()

export default function RecycledDeviceIntake({ onClose, initial, onCreated }: { onClose: () => void; initial?: { modelName: string; capacity: string; assessment: RecycledAssessment; serial: string; imei: string }; onCreated?: (id: string) => void | Promise<void> }) {
  const { t } = useDeviceTranslations()
  const copy = (key: string) => t(`deviceScan.buyback.${key}`)
  const client = useQueryClient()
  const prompt = usePrompt()
  const [step, setStep] = useState(0)
  const heading = useRef<HTMLHeadingElement>(null)
  const [modelId, setModelId] = useState("")
  const [modelSearch, setModelSearch] = useState("")
  const [capacity, setCapacity] = useState(initial?.capacity || "")
  const [variantId, setVariantId] = useState("")
  const [sourceId, setSourceId] = useState("")
  const [serial, setSerial] = useState(initial?.serial || "")
  const [imei, setImei] = useState(initial?.imei || "")
  const [cost, setCost] = useState("")
  const [bin, setBin] = useState("")
  const [notes, setNotes] = useState("")
  const [answers, setAnswers] = useState(initial?.assessment || emptyAssessment)
  const [photos, setPhotos] = useState<DevicePhoto[]>([])
  const [uploading, setUploading] = useState(false)
  const catalogue = useQuery({
    queryKey: ["buyback-models"],
    queryFn: async () => {
      const products = []
      let offset = 0
      while (true) {
        const page = await sdk.admin.product.list({ fields: "id,title,*options,*options.values", limit: 100, offset })
        products.push(...page.products.filter((product) => /^iPhone\b/i.test(product.title)))
        offset += page.products.length
        if (!page.products.length || offset >= page.count) break
      }
      return products.sort((a, b) => b.title.localeCompare(a.title, "en", { numeric: true }))
    },
  })
  const warehouses = useQuery({ queryKey: ["buyback-warehouses"], queryFn: () => sdk.client.fetch<{ sources: { id: string; name: string; type: string }[] }>("/admin/devices", { query: { category: "recycled", limit: 1 } }) })
  useEffect(() => { if (initial?.modelName && catalogue.data) setModelId(catalogue.data.find((product) => product.title === initial.modelName)?.id || "") }, [catalogue.data, initial?.modelName])
  const product = useQuery({ queryKey: ["buyback-product", modelId], queryFn: () => sdk.admin.product.retrieve(modelId, { fields: "id,title,*options,*options.values,*variants,*variants.options" }), enabled: Boolean(modelId) })
  useEffect(() => {
    if (catalogue.isError || warehouses.isError || product.isError) notifyError(t("deviceScan.loadError"))
  }, [catalogue.isError, warehouses.isError, product.isError, t])
  useEffect(() => { heading.current?.focus() }, [step])
  const sources = warehouses.data?.sources.filter((source) => source.type === "self") || []
  const warehouse = sourceId || sources[0]?.id || ""
  const model = catalogue.data?.find((item) => item.id === modelId)
  const storageOptions = model?.options?.filter((option) => /storage|capacity|memory|容量/i.test(option.title)) || []
  const capacities = [...new Set(storageOptions.flatMap((option) => option.values || []).map((item) => normalize(item.value)).filter((value) => /^\d+(GB|TB)$/.test(value)))].sort((a, b) => parseInt(a) * (a.endsWith("TB") ? 1024 : 1) - parseInt(b) * (b.endsWith("TB") ? 1024 : 1))
  const variants = (product.data?.product.variants || []).filter((variant) => variant.options?.some((option) => storageOptions.some((storage) => storage.id === option.option_id) && normalize(option.value) === capacity))
  const selectedVariantId = variantId || (variants.length === 1 ? variants[0].id : "")
  const variant = variants.find((item) => item.id === selectedVariantId)
  const batteryOption = product.data?.product.options?.find((option) => /battery|电池/i.test(option.title))
  const minimumBattery = Number(variant?.options?.find((option) => option.option_id === batteryOption?.id)?.value.replace(/[^0-9.]/g, "")) || 0
  const batteryMismatch = answers.battery !== "unknown" && validBattery(answers.battery) && Number(answers.battery) < minimumBattery
  const touchId = Boolean(model?.title.includes("SE"))
  const costValid = cost === "" || /^\d+(\.\d{1,2})?$/.test(cost) && Number(cost) <= 1000000
  const infoReady = Boolean(warehouse && model && capacity && variant && serial.trim() && isImei(imei) && costValid)
  const functionReady = validBattery(answers.battery) && !batteryMismatch && eligible(answers) && answers.checks.every((item) => item !== null)
  const appearanceReady = validMulti(answers.faults, faultIds) && validMulti(answers.glass, glassIds) && (skipScreen(answers) || gradeIds.slice(1).includes(answers.screen)) && gradeIds.includes(answers.frame) && (answers.frame === "damaged" || gradeIds.includes(answers.back))
  const savedNotes = assessmentNotes({ ...answers, priceAgreed: cost !== "" }, notes)
  const notesValid = savedNotes.length <= 2000
  const ready = notesValid && [infoReady, functionReady, appearanceReady, infoReady && !batteryMismatch && completeAssessment(answers)][step]
  const mutation = useMutation({
    mutationFn: async () => {
      if (!infoReady || batteryMismatch || !completeAssessment(answers)) throw new Error(copy("required"))
      if (savedNotes.length > 2000) throw new Error(copy("required"))
      return sdk.client.fetch<{ result?: { device_ids?: string[] } }>("/admin/devices", { method: "POST", body: { action: "intake", devices: [{
        category: "recycled", variant_id: selectedVariantId, source_id: warehouse,
        serial_number: serial.trim().toUpperCase(), imei: imei.trim(),
        ...(answers.battery === "unknown" ? {} : { battery_health: Number(answers.battery) }),
        cost_cents: Math.round(Number(cost) * 100), currency_code: "eur", bin: bin.trim(), notes: savedNotes, photos,
      }] } })
    },
    onSuccess: async (data) => {
      await client.invalidateQueries({ queryKey: ["device-lookup"] })
      toast.success(copy("success"))
      onClose()
      if (data.result?.device_ids?.[0]) await onCreated?.(data.result.device_ids[0])
    },
    onError: (error: Error) => notifyError(localizeDeviceMessage(error.message, t)),
  })
  const busy = mutation.isPending || uploading
  async function close() {
    if (busy) return
    if (modelId || serial || imei || notes || photos.length) {
      const discard = await prompt({ title: copy("leaveTitle"), description: copy("leaveHelp"), confirmText: copy("discard"), cancelText: copy("stay") })
      if (!discard) return
    }
    onClose()
  }
  const field = (label: string, child: ReactNode, help?: string) => <Label className="block min-w-0 space-y-2"><Text size="small" weight="plus">{label}</Text>{child}{help && <Text size="small" className="text-ui-fg-subtle">{help}</Text>}</Label>
  const set = <K extends keyof RecycledAssessment>(key: K, value: RecycledAssessment[K]) => setAnswers((previous) => ({ ...previous, [key]: value }))
  function multi(key: "faults" | "glass", id: string) {
    const current = answers[key]
    setAnswers({ ...answers, [key]: id === "none" ? ["none"] : current.includes(id) ? current.filter((value) => value !== id) : [...current.filter((value) => value !== "none"), id], screen: "" })
  }
  const options = (key: "screen" | "frame" | "back") => <fieldset className="space-y-3"><legend className="mb-2"><Text weight="plus">{copy(`titles.${key === "screen" ? 6 : key === "frame" ? 7 : 8}`)}</Text></legend>
    {gradeIds.filter((id) => key !== "screen" || id !== "damaged").map((id) => <label key={id} className="flex cursor-pointer items-start gap-3 rounded-md bg-ui-bg-subtle p-3 focus-within:shadow-borders-interactive-with-focus">
      <input type="radio" className="mt-1 accent-ui-fg-interactive" name={key} value={id} checked={answers[key] === id} onChange={() => setAnswers({ ...answers, [key]: id, ...(key === "frame" && id === "damaged" ? { back: "" } : {}) })} />
      <div><Text size="small" weight="plus">{copy(`grades.${gradeIds.indexOf(id)}`)}</Text><Text size="small" className="text-ui-fg-subtle">{copy(`${key}Help.${gradeIds.indexOf(id)}`)}</Text></div>
    </label>)}
  </fieldset>
  return <FocusModal open onOpenChange={(open) => { if (!open) void close() }}>
    <FocusModal.Content aria-describedby={undefined} onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }}>
      <FocusModal.Header><FocusModal.Title>{copy("title")}</FocusModal.Title><Button size="small" variant="secondary" disabled={busy} onClick={() => void close()}>{t("deviceScan.lookupClose")}</Button></FocusModal.Header>
      <FocusModal.Body className="overflow-y-auto px-6 py-4">
        <div className="mx-auto w-full max-w-3xl space-y-6" data-testid="recycled-device-intake">
          <Text size="small" className="text-ui-fg-subtle">{copy("description")}</Text>
          <ol className="grid grid-cols-2 gap-2 sm:grid-cols-4" aria-label={copy("title")}>{[0, 1, 2, 3].map((index) => <li key={index} aria-current={step === index ? "step" : undefined}><Button size="small" variant={step === index ? "primary" : "secondary"} className="h-auto w-full whitespace-normal text-left" disabled={busy || index > step} onClick={() => setStep(index)}>{index + 1}. {copy(`steps.${index}`)}</Button></li>)}</ol>
          <h2 ref={heading} tabIndex={-1} className="text-base font-medium outline-none">{copy(`steps.${step}`)}</h2>
          <fieldset disabled={busy} className="min-w-0 space-y-6">
            {step === 0 && <>
              {catalogue.isPending || warehouses.isPending ? <Text role="status">{t("deviceScan.loading")}</Text> : catalogue.isError || warehouses.isError ? <Button size="small" variant="secondary" onClick={() => { void catalogue.refetch(); void warehouses.refetch() }}>{t("orderDevices.retry")}</Button> : <>
                {field(copy("modelSearch"), <Input aria-label={copy("modelSearch")} value={modelSearch} onChange={(event) => setModelSearch(event.target.value)} />)}
                <div className="grid gap-4 sm:grid-cols-2">
                  {field(copy("titles.0"), <select className={selectClass} aria-label={copy("titles.0")} value={modelId} onChange={(event) => { setModelId(event.target.value); setCapacity(""); setVariantId(""); setAnswers(emptyAssessment()) }}><option value="">{copy("select")}</option>{catalogue.data?.filter((item) => item.id === modelId || item.title.toLowerCase().includes(modelSearch.toLowerCase())).map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}</select>, copy("help.0"))}
                  {field(copy("titles.1"), <select className={selectClass} aria-label={copy("titles.1")} value={capacity} disabled={!model} onChange={(event) => { setCapacity(event.target.value); setVariantId("") }}><option value="">{t("deviceScan.select")}</option>{capacities.map((value) => <option key={value}>{value}</option>)}</select>, model && !capacities.length ? copy("unavailable") : copy("help.1"))}
                </div>
                {!catalogue.data?.length && <Text>{copy("empty")}</Text>}
                {product.isFetching ? <Text role="status">{t("deviceScan.loading")}</Text> : product.isError ? <Button size="small" variant="secondary" onClick={() => void product.refetch()}>{t("orderDevices.retry")}</Button> : field(copy("configuration"), <select className={selectClass} aria-label={copy("configuration")} disabled={!capacity} value={selectedVariantId} onChange={(event) => setVariantId(event.target.value)}><option value="">{t("deviceScan.select")}</option>{variants.map((item) => <option key={item.id} value={item.id}>{item.title} · {item.sku || item.id}</option>)}</select>, copy("configurationHelp"))}
                <Text size="small" className="text-ui-fg-subtle">{copy("identityHelp")}</Text>
                <div className="grid gap-4 sm:grid-cols-2">
                  {field(t("orderDevices.serial"), <Input aria-label={t("orderDevices.serial")} autoComplete="off" maxLength={100} value={serial} onChange={(event) => setSerial(event.target.value.toUpperCase())} />)}
                  {field(t("orderDevices.imei"), <Input aria-label={t("orderDevices.imei")} inputMode="numeric" autoComplete="off" maxLength={15} value={imei} aria-invalid={!!imei && !isImei(imei)} onChange={(event) => setImei(event.target.value)} />, imei && !isImei(imei) ? t("deviceScan.imeiInvalid", { count: imei.length }) : undefined)}
                  {field(t("deviceScan.lookupLocation"), <select className={selectClass} aria-label={t("deviceScan.lookupLocation")} value={warehouse} onChange={(event) => setSourceId(event.target.value)}><option value="">{t("deviceScan.select")}</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}</select>)}
                  {field(t("deviceScan.bin"), <Input aria-label={t("deviceScan.bin")} maxLength={200} value={bin} onChange={(event) => setBin(event.target.value)} />)}
                </div>
                {field(copy("price"), <Input aria-label={copy("price")} type="number" min={0} max={1000000} step="0.01" value={cost} aria-invalid={!costValid} onChange={(event) => setCost(event.target.value)} />, !costValid ? t("deviceScan.editInvalidCost") : copy("costHelp"))}
              </>}
            </>}
            {step === 1 && <>
              {field(copy("titles.2"), <Input aria-label={copy("batteryInput")} inputMode="numeric" maxLength={3} disabled={answers.battery === "unknown"} value={answers.battery === "unknown" ? "" : answers.battery} onChange={(event) => set("battery", event.target.value)} aria-invalid={!!answers.battery && !validBattery(answers.battery)} />, copy("help.2"))}
              <label className="flex items-center gap-3"><input type="checkbox" checked={answers.battery === "unknown"} onChange={(event) => set("battery", event.target.checked ? "unknown" : "")} /><Text size="small">{copy("unsure")}</Text></label>
              {answers.battery && !validBattery(answers.battery) && <Text size="small" className="text-ui-fg-error">{copy("batteryError")}</Text>}
              {batteryMismatch && <Text size="small" className="text-ui-fg-error">{copy("batteryTier")}</Text>}
              {answers.battery === "unknown" && <Text size="small" className="text-ui-fg-subtle">{copy("unknownBattery")}</Text>}
              {answers.checks.map((answer, index) => {
                const key = index === 2 && touchId ? "touchId" : `checks.${index}`
                return <fieldset key={index} className="space-y-2"><legend><Text weight="plus">{copy(`${key}.0`)}</Text></legend><Text size="small" className="text-ui-fg-subtle">{copy(`${key}.1`)}</Text><div className="flex gap-4">{[true, false].map((value) => <label key={String(value)} className="flex cursor-pointer items-center gap-2"><input type="radio" name={`check-${index}`} checked={answer === value} onChange={() => set("checks", answers.checks.map((item, i) => i === index ? value : item))} /><Text size="small">{copy(value ? "yes" : "no")}</Text></label>)}</div></fieldset>
              })}
              {!eligible(answers) && <div role="status" className="space-y-2 rounded-md bg-ui-bg-subtle p-4"><Text weight="plus">{copy("cannotBuy")}</Text><Text size="small">{copy("cannotBuyHelp")}</Text></div>}
            </>}
            {step === 2 && <>
              {(["faults", "glass"] as const).map((key) => <fieldset key={key} className="space-y-3"><legend className="mb-2"><Text weight="plus">{copy(`titles.${key === "faults" ? 4 : 5}`)}</Text></legend>{[...(key === "faults" ? faultIds : glassIds), "none"].map((id, index) => <label key={id} className="flex cursor-pointer items-start gap-3 rounded-md bg-ui-bg-subtle p-3 focus-within:shadow-borders-interactive-with-focus"><input type="checkbox" className="mt-1" checked={answers[key].includes(id)} onChange={() => multi(key, id)} /><div><Text size="small" weight="plus">{copy(id === "none" ? key === "faults" ? "allWorks" : "none" : `${key}.${index}${key === "glass" ? ".0" : ""}`)}</Text>{key === "glass" && id !== "none" && <Text size="small" className="text-ui-fg-subtle">{copy(`glass.${index}.1`)}</Text>}</div></label>)}</fieldset>)}
              {!skipScreen(answers) && options("screen")}
              {options("frame")}
              {answers.frame !== "damaged" && options("back")}
              {field(t("deviceScan.intakeNotes"), <Textarea aria-label={t("deviceScan.intakeNotes")} value={notes} maxLength={900} onChange={(event) => setNotes(event.target.value)} />)}
            </>}
            {step === 3 && <>
              <div className="space-y-2 rounded-md bg-ui-bg-subtle p-4"><Heading level="h2">{model?.title} · {capacity}</Heading><Text size="small">{variant?.title}</Text><Text size="small">{t("orderDevices.serial")}: {serial}</Text><Text size="small">{t("orderDevices.imei")}: {imei}</Text><Text size="small">{t("deviceScan.lookupLocation")}: {sources.find((source) => source.id === warehouse)?.name}</Text><Text size="small">{copy("price")}: {cost ? `€${Number(cost).toFixed(2)}` : copy("pendingCost")}</Text></div>
              <RecycledAssessmentSummary value={answers} touchId={touchId} />
              {notes && <Text size="small" className="whitespace-pre-wrap">{notes}</Text>}
              <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.reviewIntakeHelp")}</Text>
            </>}
          </fieldset>
          {step === 2 && <DevicePhotosInput value={photos} onChange={setPhotos} disabled={mutation.isPending} uploading={uploading} onUploadingChange={setUploading} />}
          {!ready && <Text size="small" className="text-ui-fg-subtle">{copy("required")}</Text>}
          <div className="flex justify-between gap-3 pb-4"><Button size="small" variant="secondary" disabled={busy || step === 0} onClick={() => setStep(step - 1)}>{copy("back")}</Button><Button size="small" disabled={busy || !ready} isLoading={mutation.isPending} onClick={() => step === 3 ? mutation.mutate() : setStep(step + 1)}>{copy(step === 3 ? "save" : "next")}</Button></div>
        </div>
      </FocusModal.Body>
    </FocusModal.Content>
  </FocusModal>
}
