import {createHash,createHmac} from 'node:crypto'
import {isIP} from 'node:net'

export function customerProof(key:string,clientAddress:string,method:string,path:string,headers:Headers,body:Uint8Array|undefined,now=Date.now()):Headers{
 if(!/^[0-9a-f]{64}$/.test(key)||/^0{64}$/.test(key)||!isIP(clientAddress)||clientAddress.includes('%'))throw new Error('Customer bridge configuration unavailable')
 const timestamp=String(Math.floor(now/1000))
 const fields=['retail-customer-bridge-v1',timestamp,clientAddress,method,path,headers.get('Host')??'',headers.get('Origin')??'',headers.get('Cookie')??'',headers.get('X-CSRF-Token')??'',createHash('sha256').update(body??new Uint8Array()).digest('hex')]
 const proof=createHmac('sha256',Buffer.from(key,'hex')).update(fields.join('\n')).digest('hex')
 return new Headers({'X-Retail-Client-IP':clientAddress,'X-Retail-Bridge-Time':timestamp,'X-Retail-Bridge-Proof':proof})
}
