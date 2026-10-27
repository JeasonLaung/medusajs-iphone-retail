// Amounts are major-unit decimal strings from Gin, never minor-unit integers.
// Keep sorting and option deltas exact; currency rounding is display-only.
const decimal = /^(0|[1-9][0-9]*)(?:\.([0-9]+))?$/
export function amountParts(value: unknown): {units: bigint; scale: number} {
 if (typeof value !== 'string' || value.length > 322) throw new Error('Invalid amount')
 const match = decimal.exec(value)
 if (!match || match[1].length > 192 || (match[2]?.length ?? 0) > 128) throw new Error('Invalid amount')
 return {units: BigInt(match[1] + (match[2] ?? '')), scale: match[2]?.length ?? 0}
}
function aligned(a: string, b: string): [bigint, bigint, number] {
 const x = amountParts(a), y = amountParts(b), scale = Math.max(x.scale, y.scale)
 return [x.units * 10n ** BigInt(scale - x.scale), y.units * 10n ** BigInt(scale - y.scale), scale]
}
export function compareAmounts(a: string, b: string): -1 | 0 | 1 {
 const [x,y] = aligned(a,b)
 return x < y ? -1 : x > y ? 1 : 0
}
export function differenceAmounts(a: string, b: string): {sign: -1 | 0 | 1; amount: string} {
 const [x,y,scale] = aligned(a,b), delta = x-y, sign = delta < 0n ? -1 : delta > 0n ? 1 : 0
 const digits = (delta < 0n ? -delta : delta).toString().padStart(scale + 1,'0')
 const amount = scale ? `${digits.slice(0,-scale)}.${digits.slice(-scale)}`.replace(/\.?0+$/,'') : digits
 return {sign,amount}
}
export function percentageDecrease(original: string, calculated: string): string | null {
 const [x,y] = aligned(original,calculated)
 if (x === 0n) return null
 const numerator = (x-y)*100n, negative = numerator < 0n, absolute = negative ? -numerator : numerator
 let rounded = absolute/x
 if ((absolute%x)*2n >= x) rounded++
 return `${negative && rounded !== 0n ? '-' : ''}${rounded}`
}
export function formatAmount(amount: string, currency: string, locale = 'en-US'): string {
 amountParts(amount)
 if (!/^[a-z]{3}$/.test(currency)) throw new Error('Invalid currency')
 // Modern ECMA-402 accepts decimal strings without Number conversion. TS's
 // NumberFormat declaration still only exposes the number/bigint overload.
 const formatter = new Intl.NumberFormat(locale,{style:'currency',currency})
 return (formatter.format as unknown as (value: string) => string)(amount)
}
