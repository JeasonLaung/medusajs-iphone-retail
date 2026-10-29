"use client"

import { useEffect, useState } from "react"
import {browserCartClient,cartCSRFFromCookies} from "../../../lib/cart-client"
import {checkPaymentResult} from "../../../lib/payment-result-state"
import BrandLogo from "../../layout/brand-logo"
import { getCheckoutCopy } from "../i18n"
import styles from "./styles.module.css"

type Status = "checking" | "pending" | "failed" | "invalid" | "error" | "confirmed"
type Result = { status: Status; order_url?: string; retry_url?: string }

export default function PaymentResult({ countryCode, cartId }: { countryCode: string; cartId?: string }) {
  const copy = getCheckoutCopy(countryCode)
  const [result, setResult] = useState<Result>({ status: "checking" })
  useEffect(() => {
    let stopped = false
    let attempts = 0
    let timer: ReturnType<typeof setTimeout>
    const api=browserCartClient()
    async function check() {
      attempts++
      const checked = await checkPaymentResult(api,cartId,cartCSRFFromCookies(document.cookie,location.protocol==='https:'))
      const next: Result = {...checked,...(checked.status==='confirmed'&&checked.orderID?{order_url:`/${countryCode}/order/${encodeURIComponent(checked.orderID)}/confirmed`}:{})}
      if (stopped) return
      if (next.status === "pending" || next.status === "error") {
        if (attempts < 12) {
          setResult({ status: "pending" })
          timer = setTimeout(() => void check(), 5000)
        } else setResult({ status: "error" })
        return
      }
      setResult(next)
      if (next.status === "confirmed" && next.order_url)
        window.location.replace(next.order_url)
    }
    // Scheduling also prevents Strict Mode's effect replay from sending two
    // cart completion requests. Each retry starts after the previous one ends.
    timer = setTimeout(() => void check(), 0)
    return () => { stopped = true; clearTimeout(timer) }
  }, [cartId,countryCode])

  const messages = {
    checking: [copy.paymentCheckingTitle, copy.paymentCheckingBody],
    pending: [copy.paymentWaitingTitle, copy.paymentWaitingBody],
    failed: [copy.paymentFailedTitle, copy.paymentFailedBody],
    invalid: [copy.paymentInvalidTitle, copy.paymentInvalidBody],
    error: [copy.paymentCheckErrorTitle, copy.paymentCheckErrorBody],
    confirmed: [copy.paymentSuccessTitle, copy.paymentSuccessBody],
  }
  const [title, description] = messages[result.status]
  return <div className={styles.layout}>
    <header className={styles.header}><a href={`/${countryCode}`} aria-label="EuroRefurb"><BrandLogo /></a></header>
    <section className={styles.card} aria-labelledby="payment-result-title" data-testid="payment-result" data-status={result.status}>
      <svg className={styles.icon} viewBox="0 0 48 48" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
        <circle cx="24" cy="24" r="20" />
        {result.status === "confirmed" ? <path d="m14 24 7 7 13-14" /> :
          result.status === "checking" || result.status === "pending" ? <path d="M24 12v13l8 5" /> :
          <path d="M24 13v14m0 6v2" />}
      </svg>
      <div role="status" aria-live="polite" aria-atomic="true">
        <p className={styles.eyebrow}>{copy.paymentResultTitle}</p>
        <h1 id="payment-result-title">{title}</h1>
        <p className={styles.description}>{description}</p>
      </div>
      <div className={styles.actions}>
        {result.status === "failed" && result.retry_url &&
          <a className={styles.primary} href={result.retry_url}>{copy.paymentReturnCheckout}</a>}
        {result.status === "confirmed" && result.order_url &&
          <a className={styles.primary} href={result.order_url}>{copy.paymentViewOrder}</a>}
        {result.status !== "checking" && result.status !== "confirmed" &&
          <a className={styles.secondary} href={`/${countryCode}/help`}>{copy.paymentContactSupport}</a>}
      </div>
    </section>
  </div>
}
