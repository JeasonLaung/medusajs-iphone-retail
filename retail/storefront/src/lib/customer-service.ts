import {publicJSON} from './gin-public.ts'
import type {ServiceContact} from '../components/customer-service/contact.ts'

const unavailable=()=>new Error('Service contacts unavailable')
export async function loadServiceContacts(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<ServiceContact[]>{
 const body=await publicJSON(origin,'/api/store/customer-service',256*1024,fetcher)
 if(!body||typeof body!=='object'||!('contacts' in body)||!Array.isArray(body.contacts)||body.contacts.length>100)throw unavailable()
 const ids=new Set<string>()
 return body.contacts.map(value=>{
  if(!value||typeof value!=='object')throw unavailable()
  const row=value as Record<string,unknown>
  for(const key of ['id','name','department','whatsapp','email','timezone','start','end','availability'])if(typeof row[key]!=='string'||String(row[key]).length>1024||/[\x00-\x1f\x7f]/.test(String(row[key])))throw unavailable()
  const text=row as Record<string,string>
  if(!text.id||text.id.length>80||ids.has(text.id)||!text.name.trim()||text.name.length>100||!['pre_sales','after_sales'].includes(text.department)||!['schedule','online','offline'].includes(text.availability)||text.whatsapp&&!/^\+?[1-9]\d{6,14}$/.test(text.whatsapp)||!text.whatsapp&&!text.email||text.email&&!/^[^\s<>@]+@[^\s<>@]+\.[^\s<>@]+$/.test(text.email)||!/^([01]\d|2[0-3]):[0-5]\d$/.test(text.start)||!/^([01]\d|2[0-3]):[0-5]\d$/.test(text.end)||text.start>=text.end||!Array.isArray(row.days)||row.days.length<1||row.days.length>7||!row.days.every(day=>Number.isInteger(day)&&day>=0&&day<=6))throw unavailable()
  try{new Intl.DateTimeFormat('en',{timeZone:text.timezone})}catch{throw unavailable()}
  if(!text.timezone)throw unavailable()
  ids.add(text.id)
  return {id:text.id,name:text.name,department:text.department,whatsapp:text.whatsapp,email:text.email,timezone:text.timezone,days:[...row.days],start:text.start,end:text.end,availability:text.availability} as ServiceContact
 })
}
