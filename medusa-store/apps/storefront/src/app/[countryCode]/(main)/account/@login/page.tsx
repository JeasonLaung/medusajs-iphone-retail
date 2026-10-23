import { withSEO } from "@lib/data/seo"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { Metadata } from "next"

import LoginTemplate from "@modules/account/templates/login-template"

async function defaultPageMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return { title: t("signIn") }
}

export default async function Login() {
  return <LoginTemplate />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
