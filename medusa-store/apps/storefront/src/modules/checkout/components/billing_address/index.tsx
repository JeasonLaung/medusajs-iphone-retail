"use client"

import { HttpTypes } from "@medusajs/types"
import { useState } from "react"
import { useParams } from "next/navigation"
import AddressFields from "../address-fields"

export default function BillingAddress({ cart }: { cart: HttpTypes.StoreCart | null }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(["first_name", "last_name", "address_1", "address_2", "company", "postal_code", "city", "province", "phone", "country_code"].map((key) => [`billing_address.${key}`, String((cart?.billing_address as unknown as Record<string, unknown>)?.[key] ?? (key === "country_code" ? cart?.shipping_address?.country_code ?? countryCode.split("-")[0] : ""))])))
  return <AddressFields prefix="billing_address" values={values} onChange={(event) => setValues((current) => ({ ...current, [event.target.name]: event.target.value }))} region={cart?.region} locale={countryCode} />
}
