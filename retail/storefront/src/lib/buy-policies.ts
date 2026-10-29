import {publicJSON} from './gin-public.ts'
export type BuyPolicies={warranty_months:number;return_days:number;warranty_note:{en:string;zh:string};return_note:{en:string;zh:string}}

// Match the source storefront: failed optional policy reads show its existing
// contact-for-details state. Defaults belong to Gin's stored-setting rules.
export async function loadBuyPolicies(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<BuyPolicies|null>{
 try{
  const value=await publicJSON(origin,'/api/store/buy-policies',16*1024,fetcher)
  if(!value||typeof value!=='object'||!('policies' in value)||!value.policies||typeof value.policies!=='object')throw new Error()
  const r=value.policies as Record<string,unknown>
  if(!Number.isInteger(r.warranty_months)||Number(r.warranty_months)<0||Number(r.warranty_months)>120||!Number.isInteger(r.return_days)||Number(r.return_days)<0||Number(r.return_days)>365)throw new Error()
  const note=(raw:unknown)=>{if(!raw||typeof raw!=='object')throw new Error();const n=raw as Record<string,unknown>;if(typeof n.en!=='string'||typeof n.zh!=='string'||n.en.trim().length>200||n.zh.trim().length>200)throw new Error();return {en:n.en.trim(),zh:n.zh.trim()}}
  return {warranty_months:Number(r.warranty_months),return_days:Number(r.return_days),warranty_note:note(r.warranty_note),return_note:note(r.return_note)}
 }catch{return null}
}
