import {publicJSON,PUBLIC_NOT_FOUND} from './gin-public.ts'

type Event={timestamp:string;statusCode?:string;status?:string;description?:string;location?:{address?:{addressLocality?:string;countryCode?:string}}}
export type DhlDemo={requested_tracking_number:string;fetched_at:string;source:string;shipment:{id:string;status?:Event;events:Event[]}}
const invalid=()=>new Error('DHL demo unavailable')
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw invalid();return v as Record<string,unknown>}
const text=(v:unknown):string=>{if(typeof v!=='string')throw invalid();return v}
const optional=(v:unknown):string|undefined=>v===undefined?undefined:text(v)
function event(raw:unknown):Event{
 const e=object(raw),location=e.location===undefined?undefined:object(e.location),address=location?.address===undefined?undefined:object(location.address)
 return {timestamp:text(e.timestamp),statusCode:optional(e.statusCode),status:optional(e.status),description:optional(e.description),...(address?{location:{address:{addressLocality:optional(address.addressLocality),countryCode:optional(address.countryCode)}}}:{})}
}
export function dhlDemo(raw:unknown):DhlDemo{
 const r=object(raw),s=object(r.shipment),fetched=text(r.fetched_at),id=text(s.id)
 if(r.requested_tracking_number!=='7777777770'||r.source!=='https://developer.dhl.com/tracking'||!Number.isFinite(Date.parse(fetched))||!id||!Array.isArray(s.events))throw invalid()
 return {requested_tracking_number:r.requested_tracking_number,fetched_at:fetched,source:r.source,shipment:{id,status:s.status===undefined?undefined:event(s.status),events:s.events.map(event)}}
}
export async function loadShippingDemo(origin:string|undefined,fetcher:typeof fetch=fetch){
 // Gin allows the fixed official query 15 seconds. Keep the normal public
 // reads' five-second default, but let this bridge wait for that result.
 const raw=await publicJSON(origin,'/api/store/shipping-demo/dhl',1024*1024,fetcher,true,undefined,17000)
 return raw===PUBLIC_NOT_FOUND?PUBLIC_NOT_FOUND:dhlDemo(raw)
}
