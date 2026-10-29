"use client"

import type {CheckoutCart,CheckoutCustomer} from "./types"
import type {ContactAddress} from "../../lib/cart-client"
import type {Address} from "../../lib/customer-client"

import { useState } from "react"
import AddressFields, { CheckoutInput } from "./address-fields"
import { getCheckoutCopy } from "./i18n"
import styles from "./styles.module.css"

export default function ShippingAddress({ customer, cart, checked, onChange, segment }: {
  segment:string; customer: CheckoutCustomer | null; cart: CheckoutCart | null; checked: boolean; onChange: () => void
}) {
  const countryCode = segment
  const copy = getCheckoutCopy(countryCode)
  const initialAddress = (address?: ContactAddress | Address | null) => Object.fromEntries(["first_name", "last_name", "address_1", "address_2", "company", "postal_code", "city", "province", "phone", "country_code"].map((key) => [`shipping_address.${key}`, String((address as unknown as Record<string, unknown>)?.[key] ?? (key === "country_code" ? countryCode.split("-")[0] : ""))]))
  const [values, setValues] = useState<Record<string, string>>({ ...initialAddress(cart?.shipping_address), email: cart?.email || customer?.email || "", confirm_email: cart?.email || customer?.email || "" })
  const handleChange = (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setValues((current) => ({ ...current, [event.target.name]: event.target.value }))
  const saved = customer?.addresses.filter((address) => cart?.region?.countries?.some((country) => country.iso_2 === address.country_code)) ?? []
  return <>
    {!!saved.length && <label className={`${styles.field} mb-6`}>{copy.savedAddress}<select defaultValue="" onChange={(event) => {
      const address = saved.find((address) => address.id === event.target.value)
      if (address) setValues((current) => ({ ...current, ...initialAddress(address) }))
    }}><option value="">{copy.savedAddress}</option>{saved.map((address) => <option key={address.id} value={address.id}>{address.address_1}, {address.city}</option>)}</select></label>}
    <AddressFields prefix="shipping_address" values={values} onChange={handleChange} region={cart?.region} locale={countryCode}>
      <CheckoutInput label={copy.email} name="email" type="email" autoComplete="email" value={values.email} onChange={handleChange} full required data-testid="shipping-email-input" />
      <CheckoutInput label={copy.confirmEmail} name="confirm_email" type="email" autoComplete="off" value={values.confirm_email} onChange={handleChange} full required data-testid="confirm-email-input" />
      <div className={styles.full}><CheckoutInput label={copy.phone} name="shipping_address.phone" type="tel" autoComplete="shipping tel" value={values["shipping_address.phone"]} onChange={handleChange} required data-testid="shipping-phone-input" /><p className={styles.muted}>{copy.phoneHelp}</p></div>
    </AddressFields>
    <label className={styles.checkbox}><input type="checkbox" name="same_as_billing" checked={checked} onChange={onChange} data-testid="billing-address-checkbox" />{copy.sameBilling}</label>
  </>
}
