"use client"

import { Check, ChevronRight } from "./icons"

import type {CheckoutStep} from "./types"
import { getCheckoutCopy } from "./i18n"
import styles from "./styles.module.css"

export default function CheckoutProgress({ step, segment }: { step: CheckoutStep;segment:string }) {
  const countryCode = segment
  const pathname = `/${segment}/checkout`
  const copy = getCheckoutCopy(countryCode)
  const index = step === "address" ? 0 : step === "delivery" ? 1 : 2
  return <nav aria-label={copy.checkout} className={styles.progress}><ol>
    {[{ key: "address", label: copy.details }, { key: "delivery", label: copy.delivery }, { key: "payment", label: copy.payment }].map((item, i) => <li key={item.key} data-current={i === index} data-complete={i < index}>
      {i < index ? <a href={`${pathname}?step=${item.key}`}><span aria-hidden="true"><Check aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></span>{item.label}</a> : <div aria-current={i === index ? "step" : undefined}><span aria-hidden="true">{i + 1}</span>{item.label}</div>}
      {i < 2 && <ChevronRight aria-hidden="true" width={24} height={24} className={styles.progressSeparator} />}
    </li>)}
  </ol></nav>
}
