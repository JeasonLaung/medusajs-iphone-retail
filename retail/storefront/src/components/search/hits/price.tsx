"use client"

import { indexedCurrency } from "../client"
import {formatAmount,compareAmounts} from "../../../lib/money"
import { Text, clx } from "../ui"
import { useSearchRoute as useParams } from "../context"
import { getProductCardCopy } from "./i18n"

/** The per-currency price fields a hit may carry, e.g. `min_price_eur`. */
export type HitPricing = Record<string, unknown>

type HitPriceProps = {
  hit: HitPricing
  currencyCode: string
}


const HitPrice = ({ hit, currencyCode }: HitPriceProps) => {
  const { countryCode } = useParams()
  const copy = getProductCardCopy(countryCode)
  const currency_code = indexedCurrency(currencyCode)
  const data = hit.__gin_price as {min:string;max:string;original:string|null;on_sale:boolean}|null
  const min_price=data?.min??null
  const max_price=data?.max??null
  const original_price=data?.original??null
  const on_sale=data?.on_sale===true

  if (min_price === null) {
    return null
  }

  const format = (value: string) => formatAmount(value,currency_code)

  const max = max_price ?? min_price
  const isRange = compareAmounts(max,min_price)>0

  return (
    <div className="flex items-center gap-x-2" data-testid="product-price">
      {!isRange && on_sale && original_price !== null && (
        <Text
          className="line-through text-ui-fg-muted"
          data-testid="original-price"
        >
          {format(original_price)}
        </Text>
      )}
      <Text
        className={clx("text-ui-fg-muted", {
          "text-ui-tag-red-text": on_sale,
        })}
        data-testid="price"
      >
        {copy.fromPrice.replace("{price}", format(min_price))}
      </Text>
    </div>
  )
}

export default HitPrice
