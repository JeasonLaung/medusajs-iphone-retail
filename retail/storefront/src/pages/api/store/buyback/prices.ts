import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {buybackBridge} from '../../../../lib/buyback-bridge'
export const GET:APIRoute=({request})=>buybackBridge(request,GIN_API_ORIGIN)
