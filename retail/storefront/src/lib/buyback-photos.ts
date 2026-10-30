export type BuybackPhoto={id:string;request_id:string;item_id:string;photo_type:string;uploaded_at:string;review_status:string;review_note:string;mime_type:string}
export type BuybackPhotoUpload={item_id:string;photo_type:string;mime_type:string;content:string}
export const buybackID=(id:string)=>/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(id)
const mimeTypes=['image/jpeg','image/png','image/webp']
const fail=()=>new Error('Buyback photo unavailable')
export function photoProjection(raw:unknown,requestID:string):BuybackPhoto{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw fail()
 const r=raw as Record<string,unknown>,fields=['id','request_id','item_id','photo_type','uploaded_at','review_status','review_note','mime_type']
 if(Object.keys(r).length!==fields.length||!fields.every(k=>typeof r[k]==='string')||!buybackID(r.id as string)||r.request_id!==requestID||!buybackID(r.item_id as string)||!['front','back','edges','damage','packaging','sealed'].includes(r.photo_type as string)||!Number.isFinite(Date.parse(r.uploaded_at as string))||!['pending','approved','retake','expired'].includes(r.review_status as string)||!mimeTypes.includes(r.mime_type as string))throw fail()
 return Object.fromEntries(fields.map(k=>[k,r[k]])) as BuybackPhoto
}
export function photoContentProjection(raw:unknown):{content:string;mime_type:string}{
 if(!raw||typeof raw!=='object'||Array.isArray(raw))throw fail()
 const r=raw as Record<string,unknown>
 if(Object.keys(r).length!==2||typeof r.content!=='string'||typeof r.mime_type!=='string'||!mimeTypes.includes(r.mime_type)||r.content.length>4194304||r.content.length%4!==0||! /^[A-Za-z0-9+/]+={0,2}$/.test(r.content))throw fail()
 const b=atob(r.content),byte=(i:number)=>b.charCodeAt(i)
 const mime=byte(0)===255&&byte(1)===216&&byte(2)===255?'image/jpeg':b.slice(0,8)==='\x89PNG\r\n\x1a\n'?'image/png':b.length>=12&&b.slice(0,4)==='RIFF'&&b.slice(8,12)==='WEBP'?'image/webp':''
 if(!mime||mime!==r.mime_type||b.length>3*1024*1024)throw fail()
 return {content:r.content,mime_type:r.mime_type}
}
