"use client"

import { useEffect, useState, useTransition } from "react"
import { Alert } from "antd"
import { Button } from "./ui"
import { useAccountCopy } from "./context"
import { useCustomerActions } from "./context"

export default function VerificationNotice({ email, delivery }: { email: string; delivery: "pending" | "failed" }) {
  const { t, locale } = useAccountCopy()
  const { resendVerificationEmail } = useCustomerActions()
  const [result, setResult] = useState(delivery)
  const [seconds, setSeconds] = useState(delivery === "pending" ? 60 : 0)
  const [pending, startTransition] = useTransition()
  useEffect(() => { setResult(delivery); setSeconds(delivery === "pending" ? 60 : 0) }, [email, delivery])
  useEffect(() => {
    if (!seconds) return
    const timer = setTimeout(() => setSeconds(value => value - 1), 1000)
    return () => clearTimeout(timer)
  }, [seconds])
  return <div className="w-full mb-4" aria-live="polite">
    <Alert type={result === "pending" ? "info" : "error"} showIcon
      message={result === "pending" ? (locale.startsWith("zh") ? `验证邮件正在等待发送：${email}` : `Your verification email is waiting to be sent: ${email}`) : t("verificationSendFailed", { email })} />
    <Button className="mt-2" variant="transparent" disabled={pending || seconds > 0} onClick={() => startTransition(async () => {
      try {
        const response = await resendVerificationEmail()
        setResult(response.success ? "pending" : "failed")
      } catch { setResult("failed") }
      setSeconds(60)
    })} data-testid="resend-verification-button">
      {seconds ? t("resendCountdown", { seconds: String(seconds) }) : t("resendVerification")}
    </Button>
  </div>
}
