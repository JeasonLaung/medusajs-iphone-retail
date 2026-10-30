"use client"

import { useEffect, useRef, useState } from "react"
import { useSellNavigation } from "./navigation"
import { Alert, Button, Checkbox, Divider, Input, Popover, Radio, Select, Space, Steps, Tag, Typography } from "antd"
import FlowProgress from "./flow-progress"
import SellTheme from "../products/storefront-theme"
import { ArrowLeft, ArrowRight, InformationCircle } from "../products/icons"
import type { SellModel } from "../../lib/sell-models"
import BrandLogo from "../layout/brand-logo"
import LocalizedClientLink from "./localized-link"
import SelectionRow from "../products/selection-row"
import { getFlowCopy } from "./flow-copy"
import styles from "./flow.module.css"
import { buybackClient } from "../../lib/buyback-client"
const { estimate: getBuybackEstimate } = buybackClient()
import { getBuybackCopy } from "./buyback-copy"
import BuybackSubmission from "./buyback-submission"

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

export default function SellFlow({ models, steps, locale, customer = null }: { models: SellModel[]; steps: string[]; locale: string | null; customer?: { first_name?: string | null; last_name?: string | null; phone?: string | null } | null }) {
  const copy = getFlowCopy(locale)
  const { router, countryCode } = useSellNavigation()
  const [answers, setAnswers] = useState<Answers>(empty)
  const [ready, setReady] = useState(false)
  const [storageError, setStorageError] = useState(false)
  const [question, setQuestion] = useState(0)
  const [showAll, setShowAll] = useState(false)
  const [reference, setReference] = useState<number | null>(null)
  const [estimateLoading, setEstimateLoading] = useState(false)
  const [estimateError, setEstimateError] = useState(false)
  const [estimateProvisional, setEstimateProvisional] = useState(false)
  const [estimateDisabled, setEstimateDisabled] = useState(false)
  const [estimateRetry, setEstimateRetry] = useState(0)
  const buybackCopy = getBuybackCopy(locale)
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
  const estimate = ineligible ? copy.ineligible : estimateLoading ? buybackCopy.estimateLoading : estimateError ? buybackCopy.estimateError : estimateDisabled ? buybackCopy.estimateDisabled : reference == null ? buybackCopy.pricePending : new Intl.NumberFormat(locale?.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: "EUR" }).format(reference)
  const assessmentKey = JSON.stringify({ battery: answers.battery, checks: answers.checks, faults: answers.faults, glass: answers.glass, screen: answers.screen, frame: answers.frame, back: answers.back })
  useEffect(() => {
    let active = true
    setReference(null)
    setEstimateError(false)
    setEstimateDisabled(false)
    setEstimateProvisional(false)
    setEstimateLoading(Boolean(model?.id && memory && ready))
    const timer = setTimeout(() => {
      if (model?.id && memory && ready) getBuybackEstimate(model.id, memory, JSON.parse(assessmentKey)).then(({ estimate, error }) => {
        if (!active) return
        setReference(estimate?.amount ?? null)
        setEstimateError(Boolean(error))
        setEstimateDisabled(estimate?.status === "disabled")
        setEstimateProvisional(estimate?.provisional ?? false)
        setEstimateLoading(false)
      })
    }, 180)
    return () => { active = false; clearTimeout(timer) }
  }, [model?.id, memory, assessmentKey, ready, estimateRetry])

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
    return <Radio key={value} value={value} checked={answers[field] === value}
      onChange={() => persist(stage === "memory" && answers.memory !== value ? { ...empty(), model: model!.handle, memory: value } : { ...answers, [field]: value, ...(field === "frame" && value === "damaged" ? { back: "" } : {}) })}>
      <Space direction="vertical" size={4}><Typography.Text>{label}</Typography.Text>{description && <Typography.Text type="secondary">{description}</Typography.Text>}{common && <Tag color="blue">{copy.common}</Tag>}</Space>
    </Radio>
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
  const estimateHelp = <div className={styles.estimateHelp}>
    <p>{buybackCopy.referenceHelp}</p>
    {(!model || estimateProvisional) && <p>{buybackCopy.estimateProvisional}</p>}
  </div>
  const estimateLabel = <span className={styles.estimateLabel}>{buybackCopy.reference}{model && <Popover content={estimateHelp} trigger={["hover", "focus", "click"]} placement="top">
    <Button type="text" size="small" className={styles.estimateInfo} aria-label={buybackCopy.referenceHelpLabel} icon={<InformationCircle aria-hidden="true" />} />
  </Popover>}</span>

  return <SellTheme locale={locale}><div className={styles.page}>
    <header className={styles.header}><LocalizedClientLink href="/" aria-label="EuroRefurb"><BrandLogo /></LocalizedClientLink></header>
    <div className={styles.container}>
      {stage !== "summary" && <div className={styles.navigation}>
        <Button type="text" aria-label={copy.back} icon={<ArrowLeft aria-hidden="true" />} onClick={() => router.push(stage === "model" ? `/${countryCode}/sell` : path(stages[Math.max(0, stages.indexOf(stage) - 1)]))} />
        <FlowProgress percent={Math.round(progress / stages.length * 100)} label={copy.titles[stageIndex]} />
        <span className={styles.stepCount}>{progress}/{stages.length}</span>
      </div>}
      <div className={styles.stages}><Steps size="small" responsive current={stage === "summary" ? 3 : stageIndex < 2 ? 0 : stageIndex < 4 ? 1 : 2} items={[{ title: `${copy.titles[0]} / ${copy.titles[1]}` }, { title: `${copy.titles[2]} / ${copy.titles[3]}` }, { title: copy.condition }, { title: copy.summary }]} /></div>
      {stage === "summary" ? <div className={styles.summary}>
        <h1 ref={heading} tabIndex={-1}>{copy.summary}</h1>
        <div className={styles.estimate}>{estimateLabel}<span>{estimate}</span></div>
        {!model && <div className={styles.notice}>{estimateHelp}</div>}
        {estimateError && <Button size="small" onClick={() => setEstimateRetry((value) => value + 1)}>{buybackCopy.retry}</Button>}
        <h2>{copy.answers}</h2>
        <dl className={styles.details}>{visibleRows.slice(0, showAll ? undefined : 2).map(row => <div key={row.step} className={styles.answered}><div><dt>{row.title}</dt><dd>{row.value}</dd></div><Button type="link" onClick={() => router.push(path(row.step))} aria-label={`${copy.edit}: ${row.title}`}>{copy.edit}</Button></div>)}</dl>
        <Button type="link" onClick={() => setShowAll(!showAll)}>{showAll ? copy.hide : copy.show}</Button>
        <Button type="text" onClick={() => { persist(empty()); router.push(path("model", empty())) }}>{copy.restart}</Button>
        {model && ready && !guardedPath && !ineligible && <BuybackSubmission phone={{ model: model.handle, model_name: model.name, capacity: memory, assessment: { battery: answers.battery, checks: answers.checks, faults: answers.faults, glass: answers.glass, screen: answers.screen, frame: answers.frame, back: answers.back } }} locale={locale} customer={customer} onAddPhone={() => { persist(empty()); router.push(path("model", empty())) }} onSubmitted={(id) => { persist(empty()); router.push(`/${countryCode}/account/buyback/${id}`) }} />}
      </div> : <div className={styles.grid}>
        <aside className={styles.sidebar}>
          <div className={styles.estimate}>{estimateLabel}<span>{estimate}</span></div>
          {!model && <div className={styles.notice}>{estimateHelp}</div>}
          {estimateError && <Button size="small" onClick={() => setEstimateRetry((value) => value + 1)}>{buybackCopy.retry}</Button>}
          <dl className={styles.details}>{visibleRows.map(row => <div key={row.step}><dt>{row.title}</dt><dd>{row.value}</dd></div>)}</dl>
        </aside>
        <section className={styles.card} aria-busy={!ready}>
          <h1 ref={heading} tabIndex={-1}>{copy.titles[stageIndex]}</h1>
          <p className={styles.subtitle}>{copy.subtitles[stageIndex]}</p>
          {stageIndex < 3 && <Alert type="info" showIcon message={copy.help[stageIndex]} />}
          {stage === "model" && <div className={styles.dropdown}><Select size="large" showSearch optionFilterProp="label" value={answers.model || undefined} placeholder={copy.select} style={{ width: "100%" }} aria-label={copy.select} disabled={!ready} options={models.map(m => ({ value: m.handle, label: m.name }))} onChange={value => persist(value === answers.model ? answers : { ...empty(), model: value })} /></div>}
          {stage === "memory" && <fieldset><legend>{copy.titles[1]}</legend><Radio.Group name="memory" value={answers.memory}><Space direction="vertical" size="middle">{model?.capacities.map(c => choice(c, c))}</Space></Radio.Group>{!model?.capacities.length && <Alert type="warning" showIcon message={copy.unavailable} />}</fieldset>}
          {stage === "battery" && <div className={styles.choices}>
            <fieldset><legend>{copy.titles[2]}</legend><div className={styles.choices}>
              <SelectionRow name="battery-mode" value="known" checked={answers.battery !== "unknown"} onChange={() => persist({ ...answers, battery: "" })}>{copy.batteryInput}</SelectionRow>
              <SelectionRow name="battery-mode" value="unknown" checked={answers.battery === "unknown"} onChange={() => persist({ ...answers, battery: "unknown" })}><Space direction="vertical" size={0}><strong>{copy.unsure}</strong><Typography.Text type="secondary">{copy.unsureHelp}</Typography.Text></Space></SelectionRow>
            </div></fieldset>
            {answers.battery !== "unknown" && <div><label htmlFor="battery-capacity">{copy.batteryInput}</label><div className={styles.battery}><Input id="battery-capacity" inputMode="numeric" maxLength={3} value={answers.battery} placeholder={copy.batteryPlaceholder} suffix="%" status={answers.battery && !validBattery(answers.battery) ? "error" : undefined} aria-invalid={Boolean(answers.battery && !validBattery(answers.battery))} aria-describedby="battery-error" onChange={event => persist({ ...answers, battery: event.target.value })} /></div><p id="battery-error" role="status" className={styles.notice}>{answers.battery && !validBattery(answers.battery) ? copy.batteryError : ""}</p></div>}
          </div>}
          {stage === "functionality" && ineligible ? <div className={styles.question}>
            <Alert type="warning" showIcon message={copy.cannotBuy} description={copy.cannotBuyHelp} />
            <Button type="link" onClick={() => { const index = answers.checks[0] === false ? 0 : 1; persist({ ...answers, checks: answers.checks.map((v, i) => i < index ? v : null) }); setQuestion(index) }}>{copy.edit}</Button>
            <Button onClick={() => { persist(empty()); router.push(path("model", empty())) }}>{copy.restart}</Button>
          </div> : stage === "functionality" && <>
            {checks.slice(0, question).map((check, i) => <div className={styles.answered} key={i}><span>{check[2]}: {answers.checks[i] ? copy.yes : copy.no}</span><Button type="link" onClick={() => setQuestion(i)} aria-label={`${copy.edit}: ${check[2]}`}>{copy.edit}</Button></div>)}
            {question < checks.length && <div className={styles.question}><Tag color="blue">{question + 1}/5</Tag><h2>{checks[question][0]}</h2><p>{checks[question][1]}</p><div className={styles.yesNo}>{[true, false].map(v => <Button key={String(v)} onClick={() => { const nextChecks = [...answers.checks]; nextChecks[question] = v; persist({ ...answers, checks: nextChecks }); setQuestion(question + 1) }}>{v ? copy.yes : copy.no}</Button>)}</div></div>}
          </>}
          {(stage === "screen-faults" || stage === "screen-glass") && <fieldset><legend>{copy.titles[stageIndex]}</legend><div className={styles.choices}>
            {(stage === "screen-faults" ? faultIds : glassIds).map((id, i) => <Checkbox key={id} checked={answers[stage === "screen-faults" ? "faults" : "glass"].includes(id)} onChange={() => multi(stage === "screen-faults" ? "faults" : "glass", id)}><Space direction="vertical" size={4}><Typography.Text>{stage === "screen-faults" ? copy.faults[i] : copy.glass[i][0]}</Typography.Text>{stage === "screen-glass" && <Typography.Text type="secondary">{copy.glass[i][1]}</Typography.Text>}</Space></Checkbox>)}
            <Divider plain>{copy.or}</Divider>
            <Checkbox checked={answers[stage === "screen-faults" ? "faults" : "glass"].includes("none")} onChange={() => multi(stage === "screen-faults" ? "faults" : "glass", "none")}><strong>{stage === "screen-faults" ? copy.allWorks : copy.none}</strong></Checkbox>
          </div></fieldset>}
          {["screen", "frame", "back"].includes(stage) && <fieldset><legend>{copy.titles[stageIndex]}</legend><Radio.Group name={stage} value={answers[stage as "screen" | "frame" | "back"]}><Space direction="vertical" size="middle">{gradeIds.map((id, i) => stage === "screen" && i === 0 ? null : choice(id, copy.grades[i], (stage === "screen" ? copy.screenHelp : stage === "frame" ? copy.frameHelp : copy.backHelp)[i], stage === "screen" ? i === 3 : i === 2))}</Space></Radio.Group></fieldset>}
          {storageError && <Alert type="warning" showIcon message={copy.storageWarning} />}
          {!model && <div className={`${styles.mobileNotice} ${styles.notice}`}>{estimateHelp}</div>}
          {!(stage === "functionality" && ineligible) && <div className={styles.footer}><div className={styles.mobileEstimate}>{estimateLabel}<strong>{estimate}</strong></div><Button type="primary" size="large" disabled={!ready || !complete(stage) || stage === "functionality" && question < 5} onClick={next}>{copy.next}<ArrowRight aria-hidden="true" /></Button></div>}
        </section>
      </div>}
    </div>
  </div></SellTheme>
}
