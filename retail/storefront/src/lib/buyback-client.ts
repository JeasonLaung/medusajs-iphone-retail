export type BuybackAssessment={battery:string;checks:(boolean|null)[];faults:string[];glass:string[];screen:string;frame:string;back:string}
export type BuybackEstimate={amount:number|null;status:'estimated'|'legacy'|'unconfigured'|'disabled'|'ineligible';visual:string|null;faults:string[];provisional:boolean;currency_code:'eur'}
const fail=()=>new Error('Buyback pricing unavailable')
export function estimateProjection(raw:unknown):BuybackEstimate{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw fail()
 const r=raw as Record<string,unknown>
 if(Object.keys(r).length!==6||!Object.keys(r).every(k=>['amount','status','visual','faults','provisional','currency_code'].includes(k)))throw fail()
 return checkedEstimate(r)
}
function checkedEstimate(r:Record<string,unknown>):BuybackEstimate{
 if(!['estimated','legacy','unconfigured','disabled','ineligible'].includes(r.status as string)||r.amount!==null&&(typeof r.amount!=='number'||!Number.isFinite(r.amount)||r.amount<0)||r.visual!==null&&!['LIKE_NEW','ALMOST_NEW','GOOD','MODERATE'].includes(r.visual as string)||!Array.isArray(r.faults)||r.faults.length>5||!r.faults.every(f=>['BROKEN','BROKEN_SCREEN','BATTERY_ISSUE','BROKEN_FRAME','BROKEN_BACK_CAMERA'].includes(f))||new Set(r.faults).size!==r.faults.length||typeof r.provisional!=='boolean'||r.currency_code!=='eur')throw fail()
 if(['unconfigured','disabled','ineligible'].includes(r.status as string)&&r.amount!==null||['estimated','legacy'].includes(r.status as string)&&r.amount===null||r.status==='estimated'&&r.visual===null||r.status!=='estimated'&&(r.visual!==null||r.faults.length!==0||r.provisional!==false))throw fail()
 return {amount:r.amount as number|null,status:r.status as BuybackEstimate['status'],visual:r.visual as string|null,faults:[...r.faults],provisional:r.provisional,currency_code:'eur'}
}
export function buybackClient(fetcher:typeof fetch=fetch){return {
 async estimate(productID:string,capacity:string,assessment:BuybackAssessment):Promise<{estimate:BuybackEstimate|null;error:string|null}>{
  try{
   if(!productID||[...productID].length>100||!/^\d+(GB|TB)$/.test(capacity))throw fail()
   const response=await fetcher('/api/store/buyback/estimate',{method:'POST',credentials:'omit',redirect:'error',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify({product_id:productID,capacity,assessment})})
   if(response.status!==200)throw fail()
   return {estimate:estimateProjection(await response.json()),error:null}
  }catch{return {estimate:null,error:'BUYBACK_REQUEST_FAILED'}}
 }
}}
