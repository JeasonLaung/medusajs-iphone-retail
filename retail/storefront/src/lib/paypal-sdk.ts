import type {PayPalSDKConfig} from './cart-client'

export type WalletButtons={isEligible():boolean;render(element:HTMLElement):Promise<void>;close():Promise<void>}
type SDK={Buttons(options:{fundingSource:string;style:{layout:string;color:string;shape:string};createOrder():Promise<string>;onApprove(data:{orderID:string}):Promise<void>;onCancel():void;onError(error:unknown):void}):WalletButtons}
const scripts=new Map<string,Promise<SDK>>()
let serial=0
// Use the same v5 SDK and server create/approve contract as the original wallet.
// Isolate namespaces so a later currency change cannot consume the old SDK.
export function loadPayPalWallet(config:PayPalSDKConfig):Promise<SDK>{
 if(!/^[A-Za-z0-9_-]{1,256}$/.test(config.client_id)||!/^[A-Z]{3}$/.test(config.currency)||!['capture','authorize'].includes(config.intent)||!['sandbox','live'].includes(config.environment))return Promise.reject(Error('Invalid PayPal configuration'))
 const url=new URL('https://www.paypal.com/sdk/js')
 for(const [key,value] of Object.entries({'client-id':config.client_id,currency:config.currency,intent:config.intent,components:'buttons','disable-funding':'card,credit,paylater,venmo'}))url.searchParams.set(key,value)
 const cacheKey=url.href
 const saved=scripts.get(cacheKey);if(saved)return saved
 const promise=new Promise<SDK>((resolve,reject)=>{
  const namespace='retailPayPal'+(++serial),script=document.createElement('script')
  script.src=url.href;script.async=true;script.dataset.namespace=namespace
  const fail=()=>{clearTimeout(timer);scripts.delete(cacheKey);script.remove();reject(Error('PayPal SDK unavailable'))}
  const timer=setTimeout(fail,20000)
  script.onerror=fail
  script.onload=()=>{clearTimeout(timer);const sdk=(window as unknown as Record<string,SDK>)[namespace];if(!sdk||typeof sdk.Buttons!=='function'){fail();return}resolve(sdk)}
  document.head.append(script)
 })
 scripts.set(cacheKey,promise);return promise
}
