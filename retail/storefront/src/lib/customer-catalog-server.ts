import {GIN_API_ORIGIN,STOREFRONT_ORIGIN,CUSTOMER_BRIDGE_KEY} from 'astro:env/server'
import {customerCatalogFetcher} from './customer-catalog'

export function catalogForRequest(request:Request,readClientAddress:()=>string){
 // Internal Astro page renders can lack a socket address. Anonymous catalog
 // reads need no identity proof; authenticated reads still fail closed.
 let clientAddress=''
 if(/(?:^|;\s*)(?:__Host-)?retail_customer_session=/.test(request.headers.get('cookie')??'')){
  try{clientAddress=readClientAddress()}catch{/* Private bridge rejects missing address. */}
 }
 return customerCatalogFetcher(request,{ginOrigin:GIN_API_ORIGIN,storefrontOrigin:STOREFRONT_ORIGIN,bridgeKey:CUSTOMER_BRIDGE_KEY,clientAddress})
}
