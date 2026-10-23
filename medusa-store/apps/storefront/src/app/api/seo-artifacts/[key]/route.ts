import { getSEO } from "@lib/data/seo"
export const dynamic = "force-dynamic"
export async function GET(_req: Request, { params }: { params: Promise<{ key: string }> }) {
  const { key } = await params, { settings: values } = await getSEO()
  if (!/^[A-Za-z0-9-]{8,128}$/.test(key) || values["seo.indexnow_enabled"] !== "true" || key !== values["seo.indexnow_key"]) return new Response(null, { status: 404 })
  return new Response(key, { headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" } })
}
