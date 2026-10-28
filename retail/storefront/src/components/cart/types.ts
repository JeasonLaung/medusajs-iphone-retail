import type {CartView} from '../../lib/cart-client.ts'
import type {Variant} from '../../lib/products.ts'

export type CartDisplayLine={id:string;variant_id:string|null;title:string;product_title:string|null;product_handle:string|null;thumbnail:string|null;variant_title:string|null;quantity:number;subtotal:string}
export type CartDisplay={currency_code:string;items:CartDisplayLine[];shipping_methods:{id:string}[];shipping_subtotal:string;discount_total:string;tax_total:string;total:string}
export type CartCommands={enabled:boolean;updateLineItem:(input:{lineId:string;quantity:number})=>Promise<void>;deleteLineItem:(lineID:string)=>Promise<void>}

export function isZeroAmount(value:string):boolean{return /^-?0(?:\.0+)?$/.test(value)}
export function formatCartMoney(value:string,currency:string,locale:string):string{
 if(!/^-?(0|[1-9][0-9]{0,191})(\.[0-9]{1,128})?$/.test(value)||/[\r\n]/.test(value)||!/^[a-z]{3}$/.test(currency))throw Error('Invalid cart amount')
 const format=new Intl.NumberFormat(locale.endsWith('-zh')?'zh-CN':'en-IE',{style:'currency',currency}).format
 return (format as unknown as (value:string)=>string)(value)
}
export function variantMaximum(variant?:Variant):number|undefined{
 if(!variant?.manage_inventory||variant.allow_backorder)return undefined
 const raw=variant.availability.max_fulfillable_quantity
 if(raw===null)return 0
 if(!/^(0|[1-9][0-9]{0,191})$/.test(raw)||/[\r\n]/.test(raw))throw Error('Invalid inventory observation')
 // UI commands are bounded to 1e9. Do not convert a larger stock to Number.
 return Number(BigInt(raw)>1000000000n?1000000000n:BigInt(raw))
}
export function cartDisplay(view:CartView):CartDisplay{
 const lines=new Map(view.amounts.lines.map(line=>[line.id,line]))
 if(lines.size!==view.items.length)throw Error('Incomplete cart view')
 const items=view.items.map(item=>{
  const line=lines.get(item.id)
  if(!line||!/^[1-9][0-9]{0,9}$/.test(line.quantity)||BigInt(line.quantity)>1000000000n)throw Error('Invalid cart line')
  lines.delete(item.id)
  return {...item,variant_id:line.variant_id,quantity:Number(line.quantity),subtotal:line.totals.subtotal}
 })
 return {currency_code:view.amounts.currency_code,items,shipping_methods:view.amounts.shipping_methods,shipping_subtotal:view.amounts.totals.shipping_subtotal,discount_total:view.amounts.totals.discount_total,tax_total:view.amounts.totals.tax_total,total:view.amounts.totals.total}
}
