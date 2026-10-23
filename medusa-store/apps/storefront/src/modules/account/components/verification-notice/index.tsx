"use client"

import { useEffect, useState, useTransition } from "react"
import { Alert } from "antd"
import { Button } from "@modules/account/components/ui"
import { useAccountCopy } from "@modules/account/use-account-copy"
import { resendVerificationEmail } from "@lib/data/customer"

export default function VerificationNotice({ email, delivery }: { email: string; delivery: "queued" | "failed" }) {
  const { t } = useAccountCopy()
  const [result, setResult] = useState(delivery)
  const [seconds, setSeconds] = useState(delivery === "queued" ? 60 : 0)
  const [pending, startTransition] = useTransition()
  useEffect(() => { setResult(delivery); setSeconds(delivery === "queued" ? 60 : 0) }, [email, delivery])
  useEffect(() => {
    if (!seconds) return
    const timer = setTimeout(() => setSeconds(value => value - 1), 1000)
    return () => clearTimeout(timer)
  }, [seconds])
  return <div className="w-full mb-4" aria-live="polite">
    <Alert type={result === "queued" ? "info" : "error"} showIcon
      title={t(result === "queued" ? "verificationSent" : "verificationSendFailed", { email })} />
    <Button className="mt-2" variant="transparent" disabled={pending || seconds > 0} onClick={() => startTransition(async () => {
      try {
        const response = await resendVerificationEmail()
        setResult(response.success ? "queued" : "failed")
      } catch { setResult("failed") }
      setSeconds(60)
    })} data-testid="resend-verification-button">
      {seconds ? t("resendCountdown", { seconds: String(seconds) }) : t("resendVerification")}
    </Button>
  </div>
}
