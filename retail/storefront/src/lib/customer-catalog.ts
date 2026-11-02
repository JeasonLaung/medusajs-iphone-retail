import {customerBridge,type CustomerBridgeConfig} from './customer-bridge.ts'

// Request-local adapter for existing typed catalog loaders. Only fixed product
// and search destinations may receive customer cookies and server proof.
export function customerCatalogFetcher(request:Request,config:CustomerBridgeConfig,publicFetch:typeof fetch=fetch):typeof fetch {
 const incoming=request.headers.get('cookie')??''
 const personalized=/(?:^|;\s*)(?:__Host-)?retail_customer_session=/.test(incoming)
 return async(input,init)=>{
  const url=new URL(input instanceof Request?input.url:String(input))
  const catalog=/^\/api\/store\/products(?:\/[A-Za-z0-9][A-Za-z0-9_-]{0,190})?$/.test(url.pathname),search=url.pathname==='/api/store/search'
  if(!personalized||url.origin!==config.ginOrigin||!catalog&&!search)return publicFetch(input,init)
  if(!config.storefrontOrigin||!config.bridgeKey)throw new Error('Customer catalog unavailable')
  const target=new URL('/api/store/customer/'+(search?'search':url.pathname.slice('/api/store/'.length))+url.search,config.storefrontOrigin)
  const headers=new Headers({'Cookie':incoming,'Origin':config.storefrontOrigin,'Content-Type':'application/json'})
  const method=search?'POST':'GET'
  const response=await customerBridge(new Request(target,{method,headers,...(search?{body:init?.body}:{}),signal:init?.signal??undefined}),config)
  // An expired session is anonymous. Storage/transport failures must not hide
  // an active member's price by silently downgrading to the public quote.
  if(response.status===401)return publicFetch(input,init)
  return response
 }
}
