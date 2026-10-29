"use client"

import {
  Button,
  ErrorMessage,
  SubmitButton,
  Input,
} from "./ui"

import VerificationNotice from "./verification-notice"
import { useAccountCopy } from "./context"

import { useCustomerActions } from "./context"
import { LOGIN_VIEW } from "./login-template"
import { useActionState, useEffect, useRef } from "react"

type Props = {
  setCurrentView: (view: LOGIN_VIEW) => void
  onSuccess?: () => void
}

const Login = ({ setCurrentView, onSuccess }: Props) => {
  const { t } = useAccountCopy()
  const { login } = useCustomerActions()
  const [message, formAction] = useActionState(login, null)
  const handledSuccess = useRef(false)
  useEffect(() => {
    if (message?.state !== "success") {
      handledSuccess.current = false
    } else if (!handledSuccess.current) {
      handledSuccess.current = true
      onSuccess?.()
    }
  }, [message, onSuccess])

  return (
    <div
      className="max-w-sm w-full flex flex-col items-center"
      data-testid="login-page"
    >
      <h1 className="text-large-semi uppercase mb-6">{t("welcome")}</h1>
      <p className="text-center text-base-regular text-ui-fg-base mb-8">
        {t("loginDescription")}
      </p>
      {message?.state === "verification_required" && (
        <div className="w-full" data-testid="login-verification-message"><VerificationNotice email={message.email} delivery={message.delivery} /></div>
      )}
      <form className="w-full" action={formAction}>
        <div className="flex flex-col w-full gap-y-2">
          <Input
            label={t("email")}
            name="email"
            type="email"
            title={t("validEmail")}
            autoComplete="email"
            required
            data-testid="email-input"
          />
          <Input
            label={t("password")}
            name="password"
            type="password"
            autoComplete="current-password"
            required
            data-testid="password-input"
          />
        </div>
        <ErrorMessage
          error={message?.state === "error" ? t("authError") : null}
          data-testid="login-error-message"
        />
        <SubmitButton data-testid="sign-in-button" className="w-full mt-6">
          {t("signIn")}
        </SubmitButton>
      </form>
      <span className="text-center text-ui-fg-base text-small-regular mt-6">
        {t("notMember")}{" "}
        <Button
          variant="transparent"
          onClick={() => setCurrentView(LOGIN_VIEW.REGISTER)}
          className="underline"
          data-testid="register-button"
        >
          {t("joinUs")}
        </Button>
        .
      </span>
    </div>
  )
}

export default Login
