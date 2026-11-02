import {loadProducts,type Product} from './products.ts'

export async function loadRelatedProducts(origin:string|undefined,product:Product,regionID:string,fetcher:typeof fetch=fetch):Promise<Product[]> {
 // Preserve the source's 50-candidate window before excluding the current
 // product and restricting iPhone recommendations, then show at most 12.
 const {products}=await loadProducts(origin,{related_to:product.id,region_id:regionID,limit:50},fetcher)
 return products.filter(candidate=>candidate.id!==product.id&&(!/^iPhone\b/i.test(product.title)||/^iPhone\b/i.test(candidate.title))).slice(0,12)
}
