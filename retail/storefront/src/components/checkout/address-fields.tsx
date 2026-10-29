"use client"

import type { InputHTMLAttributes } from "react"

import type {Region} from "../../lib/regions"
import CountrySelect from "./country-select"
import { getCheckoutCopy } from "./i18n"
import styles from "./styles.module.css"

export function CheckoutInput({ label, full, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string; full?: boolean; "data-testid"?: string }) {
  return <label className={`${styles.field} ${full ? styles.full : ""}`} htmlFor={props.name}><span>{label}{props.required && <em>*</em>}</span><input {...props} id={props.name} /></label>
}

export default function AddressFields({ prefix, values, onChange, region, locale, children }: {
  prefix: "shipping_address" | "billing_address"
  values: Record<string, string>
  onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => void
  region?: Region
  locale: string
  children?: React.ReactNode
}) {
  const copy = getCheckoutCopy(locale)
  const input = (key: string, label: string, autoComplete: string, required = false, full = false) => <CheckoutInput name={`${prefix}.${key}`} label={label} autoComplete={`${prefix === "shipping_address" ? "shipping" : "billing"} ${autoComplete}`} value={values[`${prefix}.${key}`] ?? ""} onChange={onChange} required={required} full={full} data-testid={`${prefix === "shipping_address" ? "shipping" : "billing"}-${({ address_1: "address", postal_code: "postal-code", first_name: "first-name", last_name: "last-name" } as Record<string, string>)[key] ?? key}-input`} />
  return <div className={styles.fields}>
    {input("first_name", copy.firstName, "given-name", true)}{input("last_name", copy.lastName, "family-name", true)}
    {children}
    {prefix === "billing_address" && input("phone", copy.phone, "tel", false, true)}
    {input("address_1", copy.address, "address-line1", true, true)}
    <details className={styles.full} open={!!values[`${prefix}.address_2`] || !!values[`${prefix}.company`]}><summary className={styles.extraAddress}>{copy.addressExtra}</summary><div className={`${styles.fields} mt-4`}>
      {input("address_2", copy.addressExtra, "address-line2", false, true)}{input("company", copy.company, "organization", false, true)}
    </div></details>
    {input("postal_code", copy.postcode, "postal-code", true)}{input("city", copy.city, "address-level2", true)}
    <div className={`${styles.field} ${styles.full}`}><span>{copy.country}<em>*</em></span><CountrySelect segment={locale} name={`${prefix}.country_code`} region={region} value={values[`${prefix}.country_code`] ?? ""} onChange={onChange} required aria-label={copy.country} placeholder={copy.country} data-testid={`${prefix === "shipping_address" ? "shipping" : "billing"}-country-select`} /></div>
    {input("province", copy.province, "address-level1", false, true)}
  </div>
}
