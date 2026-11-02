import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {seoArtifact} from '../lib/seo-artifacts'
export const GET:APIRoute=()=>seoArtifact('robots',GIN_API_ORIGIN)
