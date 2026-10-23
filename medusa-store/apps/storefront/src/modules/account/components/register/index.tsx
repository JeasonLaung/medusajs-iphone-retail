"use client"

import {
  Button,
  Input,
  ErrorMessage,
  SubmitButton,
} from "@modules/account/components/ui"

import VerificationNotice from "@modules/account/components/verification-notice"
import { useAccountCopy } from "@modules/account/use-account-copy"

import { useActionState, useEffect, useRef } from "react"
import { LOGIN_VIEW } from "@modules/account/templates/login-template"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { signup } from "@lib/data/customer"

type Props = {
  setCurrentView: (view: LOGIN_VIEW) => void
  onSuccess?: () => void
}

const Register = ({ setCurrentView, onSuccess }: Props) => {
  const { t } = useAccountCopy()
  const [message, formAction] = useActionState(signup, null)
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
      className="max-w-sm flex flex-col items-center"
      data-testid="register-page"
    >
      <h1 className="text-large-semi uppercase mb-6">{t("registerTitle")}</h1>
      <p className="text-center text-base-regular text-ui-fg-base mb-4">
        {t("registerDescription")}
      </p>
      {message?.state === "verification_required" && (
        <div className="w-full" data-testid="register-verification-message"><VerificationNotice email={message.email} delivery={message.delivery} /></div>
      )}
      <form className="w-full flex flex-col" action={formAction}>
        <div className="flex flex-col w-full gap-y-2">
          <Input
            label={t("firstName")}
            name="first_name"
            required
            autoComplete="given-name"
            data-testid="first-name-input"
          />
          <Input
            label={t("lastName")}
            name="last_name"
            required
            autoComplete="family-name"
            data-testid="last-name-input"
          />
          <Input
            label={t("email")}
            name="email"
            required
            type="email"
            autoComplete="email"
            data-testid="email-input"
          />
          <Input
            label={t("phone")}
            name="phone"
            type="tel"
            autoComplete="tel"
            data-testid="phone-input"
          />
          <Input
            label={t("password")}
            name="password"
            required
            type="password"
            autoComplete="new-password"
            data-testid="password-input"
          />
        </div>
        <ErrorMessage
          error={message?.state === "error" ? t("registerError") : null}
          data-testid="register-error"
        />
        <span className="text-center text-ui-fg-base text-small-regular mt-6">
          {t("agreement", { privacy: "__PRIVACY__", terms: "__TERMS__" })
            .split(/(__PRIVACY__|__TERMS__)/)
            .map((part, index) =>
              part === "__PRIVACY__" ? (
                <LocalizedClientLink
                  key={index}
                  href="/content/privacy-policy"
                  className="underline"
                >
                  {t("privacy")}
                </LocalizedClientLink>
              ) : part === "__TERMS__" ? (
                <LocalizedClientLink
                  key={index}
                  href="/content/terms-of-use"
                  className="underline"
                >
                  {t("terms")}
                </LocalizedClientLink>
              ) : (
                part
              )
            )}
        </span>
        <SubmitButton className="w-full mt-6" data-testid="register-button">
          {t("join")}
        </SubmitButton>
      </form>
      <span className="text-center text-ui-fg-base text-small-regular mt-6">
        {t("alreadyMember")}{" "}
        <Button
          variant="transparent"
          onClick={() => setCurrentView(LOGIN_VIEW.SIGN_IN)}
          className="underline"
        >
          {t("signIn")}
        </Button>
        .
      </span>
    </div>
  )
}

export default Register
