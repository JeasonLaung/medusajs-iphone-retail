import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadRegions} from '../lib/regions'

export const GET:APIRoute=async()=>{
 let healthy=false
 try{await loadRegions(GIN_API_ORIGIN);healthy=true}catch{/* Dependency errors never expose configuration or response bodies. */}
 return Response.json({status:healthy?'ok':'unavailable',frontend:'astro'},{status:healthy?200:503,headers:{'Cache-Control':'no-store'}})
}
