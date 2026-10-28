import type {Variant} from './products.ts'
export type CartQuantities = Record<string,string>
const count=(value: string)=>{
 if (typeof value !== 'string' || !/^(0|[1-9][0-9]{0,191})$/.test(value)) throw new Error('Invalid cart quantity')
 return BigInt(value)
}
export function cartVariantQuantities(items: {variant_id: string | null; quantity: string}[]): CartQuantities {
 const result: CartQuantities = Object.create(null)
 for (const item of items) {
  const quantity = count(item.quantity)
  if (item.variant_id) result[item.variant_id] = (count(result[item.variant_id] ?? '0')+quantity).toString()
 }
 return result
}
// The Gin quote already includes frozen locations, shared inventory needs and
// the single-warehouse bound; pooled stock must not grant extra cart capacity.
export function cartInventoryLimitReached(variant: Variant | undefined,quantities: CartQuantities): boolean {
 if (!variant || !variant.manage_inventory || variant.allow_backorder) return false
 const maximum=variant.availability.max_fulfillable_quantity
 return maximum === null || count(quantities[variant.id] ?? '0') >= count(maximum)
}
