import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadSitemap} from '../lib/sitemap'
export const GET:APIRoute=({url})=>loadSitemap(url,GIN_API_ORIGIN)
