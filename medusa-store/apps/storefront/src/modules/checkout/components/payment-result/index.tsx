"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import BrandLogo from "@modules/common/components/brand-logo"
import { getCheckoutCopy } from "@modules/checkout/i18n"
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
    async function check() {
      attempts++
      let next: Result
      try {
        const query = cartId ? `?cart_id=${encodeURIComponent(cartId)}` : ""
        const response = await fetch(`/api/mollie-return${query}`, { method: "POST", cache: "no-store" })
        if (!response.ok) throw new Error("Payment verification unavailable")
        next = await response.json() as Result
        if (!["pending", "failed", "invalid", "error", "confirmed"].includes(next.status))
          throw new Error("Invalid payment verification response")
      } catch {
        next = { status: "error" }
      }
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
  }, [cartId])

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
    <header className={styles.header}><Link href={`/${countryCode}`} aria-label="EuroRefurb"><BrandLogo /></Link></header>
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
          <Link className={styles.primary} href={result.retry_url}>{copy.paymentReturnCheckout}</Link>}
        {result.status === "confirmed" && result.order_url &&
          <Link className={styles.primary} href={result.order_url}>{copy.paymentViewOrder}</Link>}
        {result.status !== "checking" && result.status !== "confirmed" &&
          <Link className={styles.secondary} href={`/${countryCode}/help`}>{copy.paymentContactSupport}</Link>}
      </div>
    </section>
  </div>
}
