const unavailable = () => new Error('Store data unavailable')
export const PUBLIC_NOT_FOUND = Symbol('public-resource-not-found')

// Server-owned origin only. Callers build fixed public paths; request cookies,
// browser headers and error response bodies never cross this boundary.
export async function publicJSON(origin: string | undefined, path: string, maxBytes: number, fetcher: typeof fetch = fetch, notFound = false, body?: unknown): Promise<unknown> {
 let base: URL
 try {
  if (!origin) throw unavailable()
  base = new URL(origin)
  if (!['http:','https:'].includes(base.protocol) || base.username || base.password || base.pathname !== '/' || base.search || base.hash) throw unavailable()
  const url = new URL(path,base)
  if (url.origin !== base.origin || !url.pathname.startsWith('/api/store/')) throw unavailable()
 } catch { throw unavailable() }
 const controller = new AbortController(), timer = setTimeout(() => controller.abort(),5000)
 try {
  const response = await fetcher(new URL(path,base),{cache:'no-store',credentials:'omit',redirect:'error',signal:controller.signal,headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json'})},...(body===undefined?{}:{method:'POST',body:JSON.stringify(body)})})
  if (response.status === 404 && notFound) { await response.body?.cancel().catch(()=>{}); return PUBLIC_NOT_FOUND }
  if (response.status !== 200 || response.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase() !== 'application/json' || !response.body) throw unavailable()
  const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0
  try {
   while (true) { const {done,value} = await reader.read(); if (done) break; size += value.byteLength; if (size > maxBytes) throw unavailable(); chunks.push(value) }
  } finally { await reader.cancel().catch(()=>{}); reader.releaseLock() }
  const bytes = new Uint8Array(size); let offset = 0
  for (const chunk of chunks) { bytes.set(chunk,offset); offset += chunk.length }
  return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes))
 } catch { throw unavailable() } finally { clearTimeout(timer) }
}
