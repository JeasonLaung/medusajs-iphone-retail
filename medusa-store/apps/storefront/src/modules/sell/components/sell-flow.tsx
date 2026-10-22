"use client"

import { useEffect, useRef, useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from "@headlessui/react"
import { ArrowLeft, ArrowRight, ChevronDown } from "@medusajs/icons"
import type { SellModel } from "@lib/data/sell-models"
import BrandLogo from "@modules/common/components/brand-logo"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getFlowCopy } from "../flow-copy"
import styles from "../flow.module.css"

type Answers = {
  model: string; memory: string; battery: string; checks: (boolean | null)[]
  faults: string[]; glass: string[]; screen: string; frame: string; back: string
}
const empty = (): Answers => ({ model: "", memory: "", battery: "", checks: [null, null, null, null, null], faults: [], glass: [], screen: "", frame: "", back: "" })
const key = "sell-iphone-questionnaire-v2"
const baseStages = ["model", "memory", "battery", "functionality", "screen-faults", "screen-glass", "screen", "frame", "back"]
const gradeIds = ["damaged", "visible", "some", "minimal", "none"]
const faultIds = ["bright", "pixels", "lines"]
const glassIds = ["cracked", "chipped", "scratched"]
const validBattery = (value: string) => value === "unknown" || /^\d{1,3}$/.test(value) && Number(value) >= 1 && Number(value) <= 100
const validMulti = (values: string[], allowed: string[]) => values.length > 0 && values.every((v) => allowed.includes(v)) && (!values.includes("none") || values.length === 1)

function restore(raw: string | null, models: SellModel[]): Answers {
  const result = empty()
  if (!raw) return result
  try {
    const value = JSON.parse(raw)
    const model = models.find((m) => m.handle === value.model)
    if (!model) return result
    result.model = model.handle
    if (!model.capacities.includes(value.memory)) return result
    result.memory = value.memory
    if (typeof value.battery === "string" && validBattery(value.battery)) result.battery = value.battery
    if (Array.isArray(value.checks)) result.checks = result.checks.map((_, i) => typeof value.checks[i] === "boolean" ? value.checks[i] : null)
    for (const [field, allowed] of [["faults", [...faultIds, "none"]], ["glass", [...glassIds, "none"]]] as const) {
      if (Array.isArray(value[field]) && validMulti(value[field], [...allowed])) result[field] = value[field]
    }
    for (const field of ["screen", "frame", "back"] as const) {
      if (gradeIds.includes(value[field])) result[field] = value[field]
    }
  } catch {}
  return result
}

export default function SellFlow({ models, steps, locale }: { models: SellModel[]; steps: string[]; locale: string | null }) {
  const copy = getFlowCopy(locale)
  const router = useRouter()
  const { countryCode } = useParams<{ countryCode: string }>()
  const [answers, setAnswers] = useState<Answers>(empty)
  const [ready, setReady] = useState(false)
  const [storageError, setStorageError] = useState(false)
  const [question, setQuestion] = useState(0)
  const [showAll, setShowAll] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const latest = useRef<Answers>(empty())
  const restored = useRef(false)
  const stage = steps.length === 1 ? "model" : steps.length === 2 ? "memory" : steps[2]
  const routeModel = models.find((m) => m.handle === steps[0])
  const model = routeModel ?? models.find((m) => m.handle === answers.model)
  const memory = steps.length === 3 ? steps[1].toUpperCase() : answers.memory
  const skipScreen = answers.faults.some((v) => v !== "none") || answers.glass.some((v) => v !== "none")
  const stages = baseStages.filter((s) => !(s === "screen" && skipScreen) && !(s === "back" && answers.frame === "damaged"))
  const stageIndex = baseStages.indexOf(stage)
  const progress = stage === "summary" ? stages.length : stages.indexOf(stage) + 1
  const checks = copy.checks.map((check, i) => i === 2 && model?.name.includes("SE") ? copy.touchId : check)
  const ineligible = answers.checks[0] === false || answers.checks[1] === false
  const estimate = ineligible ? copy.ineligible : "—"

  function persist(next: Answers) {
    setAnswers(next)
    latest.current = next
    try { sessionStorage.setItem(key, JSON.stringify(next)); setStorageError(false) } catch { setStorageError(true) }
  }

  function path(next: string, value = answers) {
    const root = `/${countryCode}/sell/iphone`
    if (next === "model") return `${root}/model`
    if (next === "memory") return `${root}/${value.model}/memory`
    return `${root}/${value.model}/${value.memory.toLowerCase()}/${next}`
  }

  function complete(next: string) {
    if (next === "model") return Boolean(answers.model)
    if (next === "memory") return Boolean(answers.memory)
    if (next === "battery") return validBattery(answers.battery)
    if (next === "functionality") return !ineligible && answers.checks.every((a) => a !== null)
    if (next === "screen-faults") return validMulti(answers.faults, [...faultIds, "none"])
    if (next === "screen-glass") return validMulti(answers.glass, [...glassIds, "none"])
    return gradeIds.includes(answers[next as "screen" | "frame" | "back"])
  }

  useEffect(() => {
    let value = restored.current ? latest.current : empty()
    if (!restored.current) {
      try { value = restore(sessionStorage.getItem(key), models) } catch { setStorageError(true) }
    }
    const fromUrl = models.find((m) => m.handle === steps[0])
    const urlMemory = steps.length === 3 ? steps[1].toUpperCase() : ""
    if (fromUrl && fromUrl.handle !== value.model) value = { ...empty(), model: fromUrl.handle }
    if (urlMemory && value.memory !== urlMemory) value = { ...empty(), model: value.model, memory: urlMemory }
    setAnswers(value)
    latest.current = value
    restored.current = true
    const missing = value.checks.findIndex((a) => a === null)
    setQuestion(missing === -1 ? 5 : missing)
    setReady(true)
    // Route changes preserve the tab's answers and validate the model/capacity from the URL.
  }, [models, steps])

  function guardPath() {
    if (!ready || stage === "model" || stage === "memory" || routeModel?.handle !== answers.model || memory !== answers.memory) return ""
    const previous = stages.slice(0, stage === "summary" ? stages.length : stages.indexOf(stage))
    const missing = previous.find((s) => !complete(s))
    if (missing) return path(missing)
    if (!stages.includes(stage) && stage !== "summary") return path(stage === "screen" ? "frame" : "summary")
    return ""
  }
  const guardedPath = guardPath()
  useEffect(() => { if (guardedPath) router.replace(guardedPath) }, [guardedPath, router])

  useEffect(() => { if (ready) heading.current?.focus() }, [stage, ready])

  function next() {
    if (!ready || !complete(stage)) return
    const following = stages[stages.indexOf(stage) + 1] ?? "summary"
    router.push(path(following))
  }

  function multi(field: "faults" | "glass", value: string) {
    const current = answers[field]
    const selected = value === "none" ? ["none"] : current.includes(value) ? current.filter((v) => v !== value) : [...current.filter((v) => v !== "none"), value]
    persist({ ...answers, [field]: selected, screen: "" })
  }

  function choice(value: string, label: string, description?: string, common = false) {
    const field = stage === "memory" ? "memory" : stage as "screen" | "frame" | "back"
    return <label className={styles.choice} key={value}>
      <input type="radio" name={stage} value={value} checked={answers[field] === value}
        onChange={() => persist(stage === "memory" && answers.memory !== value ? { ...empty(), model: model!.handle, memory: value } : { ...answers, [field]: value, ...(field === "frame" && value === "damaged" ? { back: "" } : {}) })} />
      <span><strong>{label}</strong>{description && <small>{description}</small>}{common && <em>{copy.common}</em>}</span>
    </label>
  }

  const detailRows = [
    { title: copy.model, value: model?.name, step: "model" },
    { title: copy.titles[1], value: memory, step: "memory" },
    { title: copy.titles[2], value: answers.battery === "unknown" ? copy.unsure : answers.battery ? `${answers.battery}%` : "", step: "battery" },
    { title: copy.titles[3], value: answers.checks.every((v) => v === true) ? copy.allWorks : checks.filter((_, i) => answers.checks[i] === false).map((c) => c[2]).join(", "), step: "functionality" },
    { title: copy.titles[4], value: answers.faults.map((v) => v === "none" ? copy.allWorks : copy.faults[faultIds.indexOf(v)]).join(", "), step: "screen-faults" },
    { title: copy.titles[5], value: answers.glass.map((v) => v === "none" ? copy.none : copy.glass[glassIds.indexOf(v)][0]).join(", "), step: "screen-glass" },
    ...["screen", "frame", "back"].filter((s) => stages.includes(s)).map((s) => ({ title: copy.titles[baseStages.indexOf(s)], value: copy.grades[gradeIds.indexOf(answers[s as "screen" | "frame" | "back"])], step: s })),
  ]
  const visibleRows = detailRows.filter((r) => r.value && (stage === "summary" || baseStages.indexOf(r.step) < stageIndex))

  return <div className={styles.page}>
    <header className={styles.header}><LocalizedClientLink href="/" aria-label="EuroRefurb"><BrandLogo /></LocalizedClientLink></header>
    <div className={styles.container}>
      {stage !== "summary" && <div className={styles.navigation}>
        <button type="button" className={styles.back} aria-label={copy.back} onClick={() => router.push(stage === "model" ? `/${countryCode}/sell` : path(stages[Math.max(0, stages.indexOf(stage) - 1)]))}><ArrowLeft aria-hidden="true" /></button>
        <div className={styles.progress} role="progressbar" aria-label={copy.titles[stageIndex]} aria-valuemin={0} aria-valuemax={stages.length} aria-valuenow={progress}><div className={styles.fill} style={{ width: `${progress / stages.length * 100}%` }} /></div>
        <span className={styles.stepCount}>{progress}/{stages.length}</span>
      </div>}
      {stage === "summary" ? <div className={styles.summary}>
        <h1 ref={heading} tabIndex={-1}>{copy.summary}</h1>
        <div className={styles.estimate}><span>{copy.estimated}</span><span>{estimate}</span></div>
        <p className={styles.notice}>{copy.quotePending}</p>
        <h2>{copy.answers}</h2>
        <dl className={styles.details}>{visibleRows.slice(0, showAll ? undefined : 2).map((row) => <div key={row.step} className={styles.answered}><span><dt>{row.title}</dt><dd>{row.value}</dd></span><button onClick={() => router.push(path(row.step))} aria-label={`${copy.edit}: ${row.title}`}>{copy.edit}</button></div>)}</dl>
        <button className={styles.textButton} onClick={() => setShowAll(!showAll)}>{showAll ? copy.hide : copy.show}</button>
        <div><button className={styles.textButton} onClick={() => { persist(empty()); router.push(path("model", empty())) }}>{copy.restart}</button></div>
      </div> : <div className={styles.grid}>
        <aside className={styles.sidebar}>
          <div className={styles.estimate}><span>{copy.estimated}</span><span>{estimate}</span></div>
          <dl className={styles.details}>{visibleRows.map((r) => <div key={r.step}><dt>{r.title}</dt><dd>{r.value}</dd></div>)}</dl>
        </aside>
        <section className={styles.card} aria-busy={!ready}>
          <h1 ref={heading} tabIndex={-1}>{copy.titles[stageIndex]}</h1>
          <p className={styles.subtitle}>{copy.subtitles[stageIndex]}</p>
          {stageIndex < 3 && <div className={styles.hint}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><circle cx="12" cy="12" r="10" /><path d="M12 10v7M12 6v2" /></svg>{copy.help[stageIndex]}</div>}
          {stage === "model" && <div className={styles.dropdown}>
            <Listbox value={answers.model} disabled={!ready} onChange={(value) => persist(value === answers.model ? answers : { ...empty(), model: value })}>
              <ListboxButton className={styles.select} aria-label={copy.select}><span>{model?.name ?? copy.select}</span><ChevronDown aria-hidden="true" /></ListboxButton>
              <ListboxOptions className={styles.options}>{models.map((m) => <ListboxOption className={styles.option} key={m.handle} value={m.handle}>{m.name}</ListboxOption>)}</ListboxOptions>
            </Listbox>
          </div>}
          {stage === "memory" && <fieldset><legend>{copy.titles[1]}</legend><div className={styles.choices}>{model?.capacities.map((c) => choice(c, c))}</div>{!model?.capacities.length && <p className={styles.notice}>{copy.unavailable}</p>}</fieldset>}
          {stage === "battery" && <div className={styles.choices}>
            <div className={styles.choice}><input type="radio" name="battery-mode" aria-label={copy.batteryInput} checked={answers.battery !== "unknown"} onChange={() => persist({ ...answers, battery: "" })} /><div>
              <label htmlFor="battery-capacity">{copy.batteryInput}</label><div className={styles.battery}><input id="battery-capacity" inputMode="numeric" maxLength={3} value={answers.battery === "unknown" ? "" : answers.battery} placeholder={copy.batteryPlaceholder} aria-invalid={Boolean(answers.battery && !validBattery(answers.battery))} aria-describedby="battery-error" onChange={(e) => persist({ ...answers, battery: e.target.value })} /><span>%</span></div>
            </div></div>
            <label className={styles.choice}><input type="radio" name="battery-mode" checked={answers.battery === "unknown"} onChange={() => persist({ ...answers, battery: "unknown" })} /><span><strong>{copy.unsure}</strong><small>{copy.unsureHelp}</small></span></label>
            <p id="battery-error" role="status" className={styles.notice}>{answers.battery && !validBattery(answers.battery) ? copy.batteryError : ""}</p>
          </div>}
          {stage === "functionality" && ineligible ? <div className={styles.question}>
            <h2>{copy.cannotBuy}</h2><p>{copy.cannotBuyHelp}</p>
            <div><button className={styles.textButton} onClick={() => { const index = answers.checks[0] === false ? 0 : 1; persist({ ...answers, checks: answers.checks.map((v, i) => i < index ? v : null) }); setQuestion(index) }}>{copy.edit}</button></div>
            <button className={styles.textButton} onClick={() => { persist(empty()); router.push(path("model", empty())) }}>{copy.restart}</button>
          </div> : stage === "functionality" && <>
            {checks.slice(0, question).map((check, i) => <div className={styles.answered} key={i}><span>{check[2]}: {answers.checks[i] ? copy.yes : copy.no}</span><button onClick={() => setQuestion(i)} aria-label={`${copy.edit}: ${check[2]}`}>{copy.edit}</button></div>)}
            {question < checks.length && <div className={styles.question}><span>{question + 1}/5</span><h2>{checks[question][0]}</h2><p>{checks[question][1]}</p><div className={styles.yesNo}>{[true, false].map((v) => <button key={String(v)} type="button" onClick={() => { const nextChecks = [...answers.checks]; nextChecks[question] = v; persist({ ...answers, checks: nextChecks }); setQuestion(question + 1) }}>{v ? copy.yes : copy.no}</button>)}</div></div>}
          </>}
          {(stage === "screen-faults" || stage === "screen-glass") && <fieldset><legend>{copy.titles[stageIndex]}</legend><div className={styles.choices}>
            {(stage === "screen-faults" ? faultIds : glassIds).map((id, i) => <label className={styles.choice} key={id}><input type="checkbox" checked={answers[stage === "screen-faults" ? "faults" : "glass"].includes(id)} onChange={() => multi(stage === "screen-faults" ? "faults" : "glass", id)} /><span><strong>{stage === "screen-faults" ? copy.faults[i] : copy.glass[i][0]}</strong>{stage === "screen-glass" && <small>{copy.glass[i][1]}</small>}</span></label>)}
            <p className={styles.or}>{copy.or}</p>
            <label className={styles.choice}><input type="checkbox" checked={answers[stage === "screen-faults" ? "faults" : "glass"].includes("none")} onChange={() => multi(stage === "screen-faults" ? "faults" : "glass", "none")} /><strong>{stage === "screen-faults" ? copy.allWorks : copy.none}</strong></label>
          </div></fieldset>}
          {["screen", "frame", "back"].includes(stage) && <fieldset><legend>{copy.titles[stageIndex]}</legend><div className={styles.choices}>{gradeIds.map((id, i) => stage === "screen" && i === 0 ? null : choice(id, copy.grades[i], (stage === "screen" ? copy.screenHelp : stage === "frame" ? copy.frameHelp : copy.backHelp)[i], stage === "screen" ? i === 3 : i === 2))}</div></fieldset>}
          {storageError && <p className={styles.notice} role="status">{copy.storageWarning}</p>}
          {!(stage === "functionality" && ineligible) && <div className={styles.footer}><div className={styles.mobileEstimate}>{copy.estimated}<strong>{estimate}</strong></div><button className={styles.primary} disabled={!ready || !complete(stage) || stage === "functionality" && question < 5} onClick={next}>{copy.next}<ArrowRight aria-hidden="true" /></button></div>}
        </section>
      </div>}
    </div>
  </div>
}
