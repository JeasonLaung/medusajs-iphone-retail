import type {Image,Product,Variant} from './products.ts'

// Adapted from the original storefront's lib/util/product-variants.ts. Option
// ordering, scoring and stable ties stay the same; availability now follows the
// authoritative Gin observation rather than legacy pooled-stock truthiness.
export const variantOptions = (variant: Variant): Record<string,string> =>
 Object.fromEntries(variant.options.filter(o=>o.option_id).map(o=>[o.option_id,o.value]))

export const variantInStock = (variant?: Variant): boolean => !!variant?.configuration_complete &&
 variant.availability.can_fulfill && ['available','backorder','untracked'].includes(variant.availability.state)

export function purchaseOptions(product: Product) {
 const order = ['storage','condition','battery','color','colour']
 const rank = (title: string) => { const index = order.indexOf(title.toLowerCase()); return index < 0 ? order.length : index }
 return [...product.options].sort((a,b)=>rank(a.title)-rank(b.title))
}
export function completeVariants(product: Product) {
 return product.variants.filter(v=>v.configuration_complete && product.options.every(o=>variantOptions(v)[o.id]))
}
export function availableOptionVariants(product: Product, selected: Record<string,string>, optionID: string, value: string) {
 const options = purchaseOptions(product), index = options.findIndex(o=>o.id === optionID)
 if (index < 0) return []
 const preceding = options.slice(0,index)
 return completeVariants(product).filter(v=>{
  const values = variantOptions(v)
  return variantInStock(v) && values[optionID] === value && preceding.every(o=>!selected[o.id] || selected[o.id] === values[o.id])
 })
}
export function preferredOptionVariant(product: Product, selected: Record<string,string>, optionID: string, value: string) {
 const score = (v: Variant) => Object.entries(variantOptions(v)).filter(([key,value])=>selected[key] === value).length
 return availableOptionVariants(product,selected,optionID,value).reduce<Variant | undefined>((best,v)=>!best || score(v) > score(best) ? v : best,undefined)
}
export function productHandleFromSlug(slug: string): string {
 const match = slug.match(/^(iphone-.+?)-\d+(?:gb|tb)-[a-z0-9-]+-[sa](?:85|100)$/i)
 return match ? `swappie-${match[1].toLowerCase()}` : slug
}
export function variantSlug(variant?: Variant): string | null {
 const slug = variant?.display.slug
 return typeof slug === 'string' && /^iphone-[a-z0-9-]+-[SA](?:85|100)$/.test(slug) ? slug : null
}
export function variantFromLocation(variants: Variant[], pathname: string, variantID: string | null, initialVariantID?: string) {
 const slug = pathname.split('/').pop()?.toLowerCase()
 return variants.find(v=>variantSlug(v)?.toLowerCase() === slug)
  ?? variants.find(v=>v.id === variantID)
  ?? variants.find(v=>v.id === initialVariantID)
}
export function initialVariant(product: Product, pathname: string, variantID: string | null, initialVariantID?: string) {
 const variants = completeVariants(product)
 return variantFromLocation(variants,pathname,variantID,initialVariantID) ?? variants.find(variantInStock) ?? variants[0]
}
export function variantForOptions(product: Product, selected: Record<string,string>) {
 return completeVariants(product).find(v=>{
  const values = variantOptions(v)
  return Object.keys(values).length === Object.keys(selected).length && Object.entries(values).every(([key,value])=>selected[key] === value)
 })
}
// The original gallery resolves its requested variant separately from purchase
// defaults. Without a requested location it keeps the model's original images.
export function galleryImages(product: Product, pathname: string, variantID: string | null, initialVariantID?: string): Image[] {
 const variant = variantFromLocation(product.variants,pathname,variantID,initialVariantID)
 return (variant?.images.length ? variant.images : product.images).filter(im=>im.url)
}
export function selectionURL(segment: string, pathname: string, search: string, hash: string, variant: Variant): string {
 if (!/^[a-z]{2}(?:-(?:en|zh))?$/.test(segment) || !pathname.startsWith('/') || pathname.startsWith('//') || /[\\\x00-\x1f\x7f?#]/.test(pathname) || hash !== '' && !hash.startsWith('#')) throw new Error('Invalid product location')
 const params = new URLSearchParams(search), slug = variantSlug(variant)
 if (slug) params.delete('v_id'); else params.set('v_id',variant.id)
 const target = slug ? `/${segment}/products/${slug}` : pathname
 return target + (params.size ? `?${params}` : '') + hash
}
export function selectionMetadata(product: Product, variant: Variant | undefined, pathname: string): {title: string; canonicalPath: string} | null {
 if (!variant || !variantSlug(variant)) return null
 return {title:`${product.title} ${variant.title} | EuroRefurb`,canonicalPath:pathname}
}
