export type User = { id: string; email: string; name: string; role: string; enabled: boolean }

export class APIError extends Error {
  constructor(readonly status: number, readonly code: string, readonly requestID = '') {
    super(code)
  }
}

function csrfCookie() {
  // Secure host cookie takes precedence; never put a session token in JS storage.
  for (const name of ['__Host-retail_admin_csrf', 'retail_admin_csrf']) {
    const matches = document.cookie.split(';').map(c => c.trim()).filter(c => c.startsWith(`${name}=`))
    if (matches.length === 1) {
      const value = matches[0].slice(name.length + 1)
      if (/^[a-f0-9]{64}$/.test(value)) return value
    }
  }
  return ''
}

async function request<T>(path: string, options: { method?: 'GET' | 'POST' | 'PATCH'; body?: unknown; csrf?: string; signal?: AbortSignal } = {}, base = '/api/admin/auth/'): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  options.signal?.addEventListener('abort', abort, { once: true })
  if (options.signal?.aborted) controller.abort()
  const timer = window.setTimeout(abort, 10_000)
  try {
    const response = await fetch(`${base}${path}`, {
      method: options.method ?? 'GET', credentials: 'same-origin', cache: 'no-store',
      redirect: 'error', signal: controller.signal,
      headers: { 'Content-Type': 'application/json', ...(options.csrf ? { 'X-CSRF-Token': options.csrf } : {}) },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    })
    const data: unknown = await response.json().catch(() => null)
    if (!response.ok) {
      const error = data && typeof data === 'object' && 'error' in data ? data.error : null
      const shape = error && typeof error === 'object' ? error as Record<string, unknown> : {}
      throw new APIError(response.status, typeof shape.code === 'string' ? shape.code : 'request.failed',
        typeof shape.request_id === 'string' && /^req_[a-f0-9-]{36}$/.test(shape.request_id) ? shape.request_id : '')
    }
    return data as T
  } catch (error) {
    if (error instanceof APIError) throw error
    throw new APIError(0, 'request.unavailable')
  } finally {
    window.clearTimeout(timer)
    options.signal?.removeEventListener('abort', abort)
  }
}

export type ProductSummary = { id: string; title: string; handle: string; status: 'draft' | 'proposed' | 'published' | 'rejected'; thumbnail: string | null; collection_id: string | null; created_at: string; updated_at: string; variant_count: number }
export type ProductList = { products: ProductSummary[]; count: number; limit: number; offset: number }
export type ProductImage = {id:string;url:string;rank:number}
export type ProductOption = {id:string;title:string;values:{id:string;value:string;rank:number|null}[]}
export type ProductDetail = ProductSummary & {revision:string;subtitle:string|null;description:string|null;options:ProductOption[];images:ProductImage[]}
export type ProductEdit = {operation_id:string;expected_revision:string;title:string;subtitle:string|null;description:string|null;status:ProductSummary['status']}
export async function editProduct(id:string,body:ProductEdit):Promise<void> {
  const csrf=csrfCookie()
  if(!csrf)throw new APIError(403,'auth.invalid_csrf')
  const result=await request<{revision:string;replayed:boolean}>(`products/${encodeURIComponent(id)}`,{method:'PATCH',csrf,body},'/api/admin/')
  if(!result||typeof result.revision!=='string'||!/^[a-f0-9]{64}$/.test(result.revision)||typeof result.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
}
export type VariantDetail = {id:string;product_id:string;title:string;sku:string|null;thumbnail:string|null;variant_rank:number|null;manage_inventory:boolean;allow_backorder:boolean;options:{id:string;option_id:string;value:string}[];unassigned_option_value_ids:string[];configuration_complete:boolean;images:ProductImage[]}
export type VariantList = {variants:VariantDetail[];count:number;limit:number;offset:number}
const nullableText=(value:unknown)=>value===null||typeof value==='string'
const imagesValid=(images:ProductImage[])=>Array.isArray(images)&&images.every(i=>i&&typeof i.id==='string'&&typeof i.url==='string'&&Number.isSafeInteger(i.rank))
const projectImages=(images:ProductImage[])=>images.map(({id,url,rank})=>({id,url,rank}))
export async function productDetail(id:string,signal:AbortSignal):Promise<ProductDetail> {
  const data=await request<{product:ProductDetail}>(`products/${encodeURIComponent(id)}`,{signal},'/api/admin/')
  const p=data?.product
  if(typeof p?.revision!=='string'||!/^[a-f0-9]{64}$/.test(p.revision))throw new APIError(0,'request.invalid_response')
  if(!p||p.id!==id||typeof p.title!=='string'||typeof p.handle!=='string'||!['draft','proposed','published','rejected'].includes(p.status)||!nullableText(p.subtitle)||!nullableText(p.description)||!nullableText(p.thumbnail)||!nullableText(p.collection_id)||typeof p.created_at!=='string'||typeof p.updated_at!=='string'||!Number.isFinite(Date.parse(p.created_at))||!Number.isFinite(Date.parse(p.updated_at))||!Number.isSafeInteger(p.variant_count)||p.variant_count<0||!Array.isArray(p.options)||!imagesValid(p.images)||!p.options.every(o=>o&&typeof o.id==='string'&&typeof o.title==='string'&&Array.isArray(o.values)&&o.values.every(v=>v&&typeof v.id==='string'&&typeof v.value==='string'&&(v.rank===null||Number.isSafeInteger(v.rank)))))throw new APIError(0,'request.invalid_response')
  return {id:p.id,title:p.title,handle:p.handle,status:p.status,thumbnail:p.thumbnail,collection_id:p.collection_id,created_at:p.created_at,updated_at:p.updated_at,variant_count:p.variant_count,revision:p.revision,subtitle:p.subtitle,description:p.description,images:projectImages(p.images),options:p.options.map(({id,title,values})=>({id,title,values:values.map(({id,value,rank})=>({id,value,rank}))}))}
}
export async function productVariants(id:string,query:URLSearchParams,signal:AbortSignal):Promise<VariantList>{
  const data=await request<VariantList>(`products/${encodeURIComponent(id)}/variants?${query}`,{signal},'/api/admin/')
  if(!data||!Number.isSafeInteger(data.count)||data.count<0||!Number.isSafeInteger(data.limit)||data.limit<1||data.limit>100||!Number.isSafeInteger(data.offset)||data.offset<0||!Array.isArray(data.variants)||data.variants.length>data.limit||!data.variants.every(v=>v&&typeof v.id==='string'&&v.product_id===id&&typeof v.title==='string'&&nullableText(v.sku)&&nullableText(v.thumbnail)&&(v.variant_rank===null||Number.isSafeInteger(v.variant_rank))&&typeof v.manage_inventory==='boolean'&&typeof v.allow_backorder==='boolean'&&typeof v.configuration_complete==='boolean'&&Array.isArray(v.unassigned_option_value_ids)&&v.unassigned_option_value_ids.every(x=>typeof x==='string')&&imagesValid(v.images)&&Array.isArray(v.options)&&v.options.every(o=>o&&typeof o.id==='string'&&typeof o.option_id==='string'&&typeof o.value==='string')))throw new APIError(0,'request.invalid_response')
  return {count:data.count,limit:data.limit,offset:data.offset,variants:data.variants.map(v=>({id:v.id,product_id:v.product_id,title:v.title,sku:v.sku,thumbnail:v.thumbnail,variant_rank:v.variant_rank,manage_inventory:v.manage_inventory,allow_backorder:v.allow_backorder,configuration_complete:v.configuration_complete,unassigned_option_value_ids:[...v.unassigned_option_value_ids],options:v.options.map(({id,option_id,value})=>({id,option_id,value})),images:projectImages(v.images)}))}
}
export async function listProducts(query: URLSearchParams, signal: AbortSignal): Promise<ProductList> {
  const data = await request<ProductList>(`products?${query}`, { signal }, '/api/admin/')
  if (!data || !Number.isSafeInteger(data.count) || data.count < 0 || !Number.isSafeInteger(data.limit) || data.limit < 1 || data.limit > 100 || !Number.isSafeInteger(data.offset) || data.offset < 0 || !Array.isArray(data.products) || data.products.length > data.limit) throw new APIError(0, 'request.invalid_response')
  const products = data.products.map(p => {
    if (!p || typeof p.id !== 'string' || !p.id || typeof p.title !== 'string' || typeof p.handle !== 'string' || !['draft','proposed','published','rejected'].includes(p.status) || !Number.isSafeInteger(p.variant_count) || p.variant_count < 0 || typeof p.created_at !== 'string' || typeof p.updated_at !== 'string' || !Number.isFinite(Date.parse(p.created_at)) || !Number.isFinite(Date.parse(p.updated_at)) || (p.thumbnail !== null && typeof p.thumbnail !== 'string') || (p.collection_id !== null && typeof p.collection_id !== 'string')) throw new APIError(0, 'request.invalid_response')
    return { id:p.id,title:p.title,handle:p.handle,status:p.status,thumbnail:p.thumbnail,collection_id:p.collection_id,created_at:p.created_at,updated_at:p.updated_at,variant_count:p.variant_count }
  })
  return { products, count:data.count,limit:data.limit,offset:data.offset }
}

function account(data: unknown): User {
  if (!data || typeof data !== 'object' || !('user' in data)) throw new APIError(0, 'request.invalid_response')
  const u = data.user as Partial<User> | null
  if (!u || typeof u.id !== 'string' || !u.id || typeof u.email !== 'string' || typeof u.name !== 'string' || u.role !== 'admin' || u.enabled !== true) {
    throw new APIError(0, 'request.invalid_response')
  }
  return { id: u.id, email: u.email, name: u.name, role: u.role, enabled: u.enabled }
}

export const auth = {
  async me(signal?: AbortSignal) { return account(await request('me', { signal })) },
  async login(email: string, password: string) {
    // Obtain a fresh one-use challenge for every attempt. Never retry a write.
    const challenge = await request<{ csrf_token: string }>('challenge', { method: 'POST' })
    if (!challenge || !/^[a-f0-9]{64}$/.test(challenge.csrf_token)) throw new APIError(0, 'request.invalid_response')
    return account(await request('login', { method: 'POST', csrf: challenge.csrf_token, body: { email, password } }))
  },
  async logout() {
    const csrf = csrfCookie()
    if (!csrf) throw new APIError(403, 'auth.invalid_csrf')
    const result = await request<{ success: boolean }>('logout', { method: 'POST', csrf })
    if (result?.success !== true) throw new APIError(0, 'request.invalid_response')
  },
}
