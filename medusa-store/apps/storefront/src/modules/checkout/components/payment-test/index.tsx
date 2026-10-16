"use client"

import { Badge } from "@modules/common/components/ui"
import { useParams } from "next/navigation"
import { getCheckoutCopy } from "../../i18n"

export default function PaymentTest({ className }: { className?: string }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  return <Badge color="orange" className={className}>{getCheckoutCopy(countryCode).testingOnly}</Badge>
}
