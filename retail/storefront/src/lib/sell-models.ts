import {loadProducts,type Product} from './products.ts'

export type SellModel={id:string;name:string;handle:string;capacities:string[]}
type SellProduct=Pick<Product,'id'|'title'|'handle'|'options'>

// Original sell-models policy: eligibility comes from the catalogue, not from
// configured buyback prices. Keep title replacement and capacity normalization.
export function sellModels(products:SellProduct[]):SellModel[]{
 const models=new Map<string,SellModel>()
 for(const product of products){
  const title=product.title.trim()
  if(!/^iPhone\b/i.test(title)||!product.handle)continue
  const capacities=product.options.filter(option=>/storage|capacity|memory|容量/i.test(option.title))
   .flatMap(option=>option.values).map(({value})=>value.replace(/\s+/g,'').toUpperCase()).filter(value=>/^\d+(GB|TB)$/.test(value))
  models.set(title,{id:product.id,name:title,handle:product.handle.replace(/^swappie-/,''),capacities:Array.from(new Set(capacities)).sort((a,b)=>parseInt(a)*(a.endsWith('TB')?1024:1)-parseInt(b)*(b.endsWith('TB')?1024:1))})
 }
 const rank=(name:string)=>{
  if(name==='iPhone Air')return 173
  const generation=Number(name.match(/^iPhone (\d+)/)?.[1]??0)
  const variant=name.includes('Pro Max')?5:name.includes('Pro')?4:name.includes('Plus')?3:name.includes('mini')?0:/\de$/.test(name)?1:2
  return generation*10+variant
 }
 return Array.from(models.values()).sort((a,b)=>rank(b.name)-rank(a.name)||b.name.localeCompare(a.name,'en',{numeric:true}))
}

export async function loadSellModels(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<SellModel[]>{
 const products:SellProduct[]=[],seen=new Set<string>()
 let offset=0,total:number|undefined
 while(true){
  // Smaller pages share the established public projection/relation budget.
  const page=await loadProducts(origin,{limit:12,offset},fetcher)
  if(total!==undefined&&page.count!==total||page.count>100000)throw new Error('Sell catalogue unavailable')
  total=page.count
  for(const product of page.products){
   if(seen.has(product.id))throw new Error('Sell catalogue unavailable')
   seen.add(product.id);products.push(product)
  }
  offset+=page.products.length
  if(offset>=total)break
  if(page.products.length===0)throw new Error('Sell catalogue unavailable')
 }
 return sellModels(products)
}
