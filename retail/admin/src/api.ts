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

async function request<T>(path: string, options: { method?: 'GET' | 'POST'; body?: unknown; csrf?: string; signal?: AbortSignal } = {}, base = '/api/admin/auth/'): Promise<T> {
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
