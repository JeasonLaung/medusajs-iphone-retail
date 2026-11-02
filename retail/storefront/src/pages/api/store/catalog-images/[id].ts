import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {publicCatalogImage} from '../../../../lib/catalog-image'
export const GET:APIRoute=({params,url})=>publicCatalogImage(GIN_API_ORIGIN,params.id,url.search)
