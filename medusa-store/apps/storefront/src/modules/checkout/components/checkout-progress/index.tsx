"use client"

import { useParams, usePathname } from "next/navigation"
import Link from "next/link"
import { CheckoutStep } from "@lib/util/checkout-step"
import { getCheckoutCopy } from "../../i18n"
import styles from "../../styles.module.css"

export default function CheckoutProgress({ step }: { step: CheckoutStep }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const pathname = usePathname()
  const copy = getCheckoutCopy(countryCode)
  const index = step === "address" ? 0 : step === "delivery" ? 1 : 2
  return <nav aria-label={copy.checkout} className={styles.progress}><ol>
    {[{ key: "address", label: copy.details }, { key: "delivery", label: copy.delivery }, { key: "payment", label: copy.payment }].map((item, i) => <li key={item.key} data-current={i === index} data-complete={i < index}>
      {i < index ? <Link href={`${pathname}?step=${item.key}`}><span aria-hidden="true">{"✓"}</span>{item.label}</Link> : <div aria-current={i === index ? "step" : undefined}><span aria-hidden="true">{i + 1}</span>{item.label}</div>}
    </li>)}
  </ol></nav>
}
