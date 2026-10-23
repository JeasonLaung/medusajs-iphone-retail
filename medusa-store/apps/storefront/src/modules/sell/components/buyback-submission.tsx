"use client"

import { useEffect, useState } from "react"
import { Alert, Button, Checkbox, Form, Input, Modal } from "antd"
import { useParams } from "next/navigation"
import { createBuybackRequest, type BuybackAssessment, type BuybackDraft } from "@lib/data/buyback"
import { getBuybackCopy } from "../buyback-copy"
import LoginTemplate from "@modules/account/templates/login-template"
import styles from "../request.module.css"

type DraftPhone = { model: string; model_name: string; capacity: string; assessment: BuybackAssessment }
const draftKey = "buyback-phones-v1"
const submissionKey = "buyback-submission-key-v1"
export default function BuybackSubmission({ phone, locale, customer, onAddPhone, onSubmitted }: {
  phone: DraftPhone; locale: string | null; customer: { first_name?: string | null; last_name?: string | null; phone?: string | null } | null
  onAddPhone: () => void; onSubmitted: (id: string) => void
}) {
  const copy = getBuybackCopy(locale)
  const { countryCode } = useParams<{ countryCode: string }>()
  const [drafts, setDrafts] = useState<DraftPhone[]>([])
  const [key, setKey] = useState("")
  const [ready, setReady] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [authOpen, setAuthOpen] = useState(false)
  const [authenticated, setAuthenticated] = useState(Boolean(customer))
  const [form] = Form.useForm()
  useEffect(() => {
    try {
      const stored = JSON.parse(sessionStorage.getItem(draftKey) || "[]")
      if (Array.isArray(stored)) setDrafts(stored.filter((item) => item?.model && item.capacity && Array.isArray(item.assessment?.checks)).slice(0, 9))
      const value = sessionStorage.getItem(submissionKey) || crypto.randomUUID()
      sessionStorage.setItem(submissionKey, value)
      setKey(value)
    } catch { setKey(crypto.randomUUID()) }
    setReady(true)
  }, [])
  function store(next: DraftPhone[]) {
    try { sessionStorage.setItem(draftKey, JSON.stringify(next)); setDrafts(next); return true }
    catch { setError(copy.errors.BUYBACK_REQUEST_FAILED); return false }
  }
  async function submit(values: Record<string, string | boolean>, justAuthenticated = false) {
    if (busy || !key) return
    if (!authenticated && !justAuthenticated) {
      setAuthOpen(true)
      return
    }
    setBusy(true)
    setError("")
    const value = (key: string) => String(values[key] || "").trim()
    const body: BuybackDraft = {
      submission_key: key, contact_name: value("name"), phone: value("phone"), reference_acknowledged: true, privacy_acknowledged: true,
      return_address: { name: value("name"), address_1: value("address"), address_2: value("address2"), city: value("city"), postal_code: value("postal"), country_code: value("country").toLowerCase() },
      items: [...drafts, phone].map(({ model, capacity, assessment }) => ({ model, capacity, assessment })),
    }
    try {
      const result = await createBuybackRequest(body)
      if (result.data) {
        try { sessionStorage.removeItem(draftKey); sessionStorage.removeItem(submissionKey) } catch {}
        onSubmitted(result.data.request.id)
      } else if (result.error === "BUYBACK_LOGIN_REQUIRED") {
        setAuthenticated(false)
        setAuthOpen(true)
      } else setError(copy.errors[result.error as keyof typeof copy.errors] || copy.errors.BUYBACK_REQUEST_FAILED)
    } catch { setError(copy.errors.BUYBACK_REQUEST_FAILED) }
    finally { setBusy(false) }
  }
  return <section className={styles.section} data-testid="buyback-submission">
    <h2>{copy.newRequest}</h2><p className={styles.notice}>{copy.referenceHelp}</p>
    <h3>{copy.devices}</h3>
    {[...drafts, phone].map((item, index) => <div key={index} className={styles.row}><span>{index + 1}. {item.model_name} · {item.capacity}</span>{index < drafts.length && <Button type="link" danger disabled={busy} onClick={() => store(drafts.filter((_, i) => index !== i))}>{copy.remove}</Button>}</div>)}
    <Button disabled={!ready || busy || drafts.length >= 9} onClick={() => { if (store([...drafts, phone])) onAddPhone() }}>{copy.nextPhone}</Button>
    <div className={styles.form}><Form form={form} layout="vertical" onFinish={(values) => submit(values)} disabled={busy || authOpen} requiredMark initialValues={{ name: `${customer?.first_name || ""} ${customer?.last_name || ""}`.trim(), phone: customer?.phone || "" }}>
      <h3>{copy.contact}</h3><p className={styles.notice}>{copy.originalAddress}</p>
      <div className={styles.grid}>
        {([ ["name", copy.name, "name", `${customer?.first_name || ""} ${customer?.last_name || ""}`.trim(), true], ["phone", copy.phone, "tel", customer?.phone || "", true], ["address", copy.address, "address-line1", "", true], ["address2", copy.address2, "address-line2", "", false], ["city", copy.city, "address-level2", "", true], ["postal", copy.postal, "postal-code", "", true], ["country", copy.country, "country", "", true] ] as const).map(([name, label, autocomplete, , required]) => <Form.Item key={name} name={name} label={label} rules={[{ required, whitespace: true, message: copy.required }, { min: name === "name" ? 2 : name === "phone" ? 5 : name === "address" ? 3 : 1, message: copy.invalidField }, ...(name === "country" ? [{ pattern: /^[A-Za-z]{2}$/, message: copy.invalidCountry }] : [])]}><Input autoComplete={autocomplete} type={name === "phone" ? "tel" : "text"} maxLength={name === "country" ? 2 : name === "phone" ? 50 : name === "postal" ? 30 : 100} /></Form.Item>)}
      </div>
      <Form.Item name="reference" valuePropName="checked" rules={[{ validator: (_, value) => value ? Promise.resolve() : Promise.reject(new Error(copy.required)) }]}><Checkbox>{copy.acknowledge}</Checkbox></Form.Item>
      <p className={styles.notice}>{copy.privacy}</p>
      <Form.Item name="privacy" valuePropName="checked" rules={[{ validator: (_, value) => value ? Promise.resolve() : Promise.reject(new Error(copy.required)) }]}><Checkbox>{copy.privacyAgree}</Checkbox></Form.Item>
      {error && <Alert type="error" showIcon title={error} />}
      <div className={styles.actions}><Button type="primary" disabled={!ready} loading={busy} htmlType="submit">{busy ? copy.submitting : copy.submit}</Button><Button type="link" href={`/${countryCode}/account/buyback`}>{copy.myRequests}</Button></div>
    </Form></div>
    <Modal open={authOpen} title={copy.authTitle} footer={null} onCancel={() => setAuthOpen(false)} destroyOnHidden width={480} centered styles={{ body: { maxHeight: "calc(100dvh - 160px)", overflowY: "auto" } }}>
      <p className={styles.notice}>{copy.authHelp}</p>
      <LoginTemplate onSuccess={() => {
        setAuthenticated(true)
        setAuthOpen(false)
        void submit(form.getFieldsValue(), true)
      }} />
    </Modal>
  </section>
}
