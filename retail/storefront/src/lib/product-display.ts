import {amountParts} from './money.ts'

export type DisplayPrice = {amount: string; currency_code: string}
export const specFields = ['release_year','display_size_inches','display_name','display_resolution_px','pixel_density_ppi','chip','rear_camera_system','rear_camera_count','front_camera','height_mm','width_mm','depth_mm','weight_g','connector','biometric','tech_specs_url'] as const
export type ModelSpecs = Partial<Record<typeof specFields[number],string>> & {apple_model_numbers?: string[]}
export type ProductDisplay = {colors: {name: string; hex: string}[]; model_specs: ModelSpecs | null; launch_prices: (DisplayPrice & {storage: string})[]; source: 'swappie-html-snapshot' | null}
export type VariantDisplay = {slug: string | null; demo_price_basis: 'model-starting-price' | null; apple_launch_price: DisplayPrice | null}
const invalid = () => new Error('Store display unavailable')
function object(v: unknown): Record<string,unknown> { if (!v || typeof v !== 'object' || Array.isArray(v)) throw invalid(); return v as Record<string,unknown> }
function text(v: unknown): string { if (typeof v !== 'string' || new TextEncoder().encode(v).length > 4096 || /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v)) throw invalid(); return v }
function array(v: unknown, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) throw invalid(); return v }
function price(v: unknown): DisplayPrice {
 const p = object(v), amount = text(p.amount), currency = text(p.currency_code)
 if (amountParts(amount).units === 0n || !/^[a-z]{3}$/.test(currency)) throw invalid()
 return {amount,currency_code:currency}
}
export function projectProductDisplay(input: unknown): ProductDisplay {
 const p = object(input), source = p.source
 if (source !== null && source !== 'swappie-html-snapshot') throw invalid()
 const colors = array(p.colors,64).map(input => {
  const c = object(input), name = text(c.name), hex = text(c.hex)
  if (!name || Array.from(name).length > 191 || !/^#[0-9a-fA-F]{6}$/.test(hex)) throw invalid()
  return {name,hex}
 })
 let specs: ModelSpecs | null = null
 if (p.model_specs !== null) {
  const raw = object(p.model_specs); specs = {}
  for (const key of specFields) if (raw[key] !== undefined) specs[key] = text(raw[key])
  if (specs.tech_specs_url !== undefined && !/^https:\/\/(support\.)?apple\.com\//.test(specs.tech_specs_url)) throw invalid()
  if (raw.apple_model_numbers !== undefined) specs.apple_model_numbers = array(raw.apple_model_numbers,64).map(text)
 }
 const launchPrices = array(p.launch_prices,128).map(input => {
  const v = object(input), storage = text(v.storage)
  if (!storage || Array.from(storage).length > 191 || /[\x00-\x1f\x7f]/.test(storage)) throw invalid()
  return {storage,...price(v)}
 })
 if (new Set(launchPrices.map(p=>p.storage)).size !== launchPrices.length) throw invalid()
 return {colors,model_specs:specs,launch_prices:launchPrices,source}
}
export function projectVariantDisplay(input: unknown): VariantDisplay {
 const v = object(input), slug = v.slug === null ? null : text(v.slug), basis = v.demo_price_basis
 if (slug !== null && (Array.from(slug).length > 191 || !/^iphone-[a-z0-9-]+-[SA](85|100)$/.test(slug)) || basis !== null && basis !== 'model-starting-price') throw invalid()
 return {slug,demo_price_basis:basis,apple_launch_price:v.apple_launch_price === null ? null : price(v.apple_launch_price)}
}
