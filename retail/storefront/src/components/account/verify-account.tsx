"use client"

import {Button} from "./ui"
import LocalizedLink from "./localized-link"

import { Result, Spin } from "antd"
import StorefrontTheme from "../products/storefront-theme"
import { useAccountCopy } from "./context"

import { useEffect, useRef, useState } from "react"

import {useCustomerActions} from "./context"

type VerificationState = "verifying" | "success" | "error"

const VerifyAccount = () => {
  const { t, locale } = useAccountCopy()
  const {confirmEmailVerification} = useCustomerActions()
  const [token] = useState(()=>typeof window==="undefined"?null:new URL(window.location.href).searchParams.get("token"))
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
              <LocalizedLink href="/account"><Button>{t("goSignIn")}</Button></LocalizedLink>
            )
          }
        />
      </div>
    </StorefrontTheme>
  )
}

export default VerifyAccount
