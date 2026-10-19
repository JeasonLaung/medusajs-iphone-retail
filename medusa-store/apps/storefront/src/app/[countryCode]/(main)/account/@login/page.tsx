import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { Metadata } from "next"

import LoginTemplate from "@modules/account/templates/login-template"

export async function generateMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return { title: t("signIn") }
}

export default async function Login() {
  const { t } = getAccountCopy(await getLocale())
  return <LoginTemplate />
}
