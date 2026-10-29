import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN,STOREFRONT_ORIGIN} from 'astro:env/server'
import {cartBridge} from '../../../lib/cart-bridge'
export const GET:APIRoute=({request})=>cartBridge(request,{ginOrigin:GIN_API_ORIGIN,storefrontOrigin:STOREFRONT_ORIGIN})
export const POST:APIRoute=GET
