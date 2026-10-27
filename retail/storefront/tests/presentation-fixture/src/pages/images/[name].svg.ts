import type {APIRoute} from 'astro'
// Local deterministic artificial image: no remote download or business image.
export const GET:APIRoute=({params})=>new Response(`<svg xmlns="http://www.w3.org/2000/svg" width="300" height="420" viewBox="0 0 300 420"><rect x="30" y="10" width="240" height="400" rx="30" fill="#1677FF"/><text x="150" y="210" text-anchor="middle" fill="white">${(params.name??'').replace(/[^a-z0-9-]/g,'')}</text></svg>`,{headers:{'content-type':'image/svg+xml'}})
