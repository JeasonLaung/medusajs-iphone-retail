"use client"

import { LinkButton } from "@modules/account/components/ui"

import { Result, Spin } from "antd"
import StorefrontTheme from "@modules/common/components/storefront-theme"
import { useAccountCopy } from "@modules/account/use-account-copy"

import { useEffect, useRef, useState } from "react"
import { useSearchParams } from "next/navigation"
import { confirmEmailVerification } from "@lib/data/customer"

type VerificationState = "verifying" | "success" | "error"

const VerifyAccount = () => {
  const { t, locale } = useAccountCopy()
  const searchParams = useSearchParams()
  const token = searchParams.get("token")
  const [state, setState] = useState<VerificationState>("verifying")
  // Guard against the effect running twice in React Strict Mode, which would
  // consume the single-use token before the customer sees the result.
  const confirmed = useRef<string | null | undefined>(undefined)

  useEffect(() => {
    if (confirmed.current === token) {
      return
    }
    confirmed.current = token
    setState("verifying")

    if (!token) {
      setState("error")
      return
    }

    confirmEmailVerification(token).then(({ success }) => {
      if (confirmed.current === token) setState(success ? "success" : "error")
    }).catch(() => { if (confirmed.current === token) setState("error") })
  }, [token])

  return (
    <StorefrontTheme locale={locale}>
      <div className="w-full" data-testid="verify-account-page">
        <Result
          status={
            state === "success"
              ? "success"
              : state === "error"
              ? "error"
              : "info"
          }
          icon={state === "verifying" ? <Spin size="large" /> : undefined}
          title={t("verificationTitle")}
          subTitle={t(
            state === "verifying"
              ? "verifying"
              : state === "success"
              ? "verified"
              : "invalidVerification"
          )}
          extra={
            state !== "verifying" && (
              <LinkButton href="/account">{t("goSignIn")}</LinkButton>
            )
          }
        />
      </div>
    </StorefrontTheme>
  )
}

export default VerifyAccount
