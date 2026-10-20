import { sdk } from "./sdk"

export async function createMobileIntakeLink(sourceId: string, category: "sale" | "returned" | "recycled", language: string, origin = window.location.origin) {
  const base = new URL(origin)
  if (!["http:", "https:"].includes(base.protocol) || base.username || base.password) throw new Error("mobileIntake.originError")
  const result = await sdk.client.fetch<{ token: string; expires_at: number }>("/admin/devices/mobile-intake", {
    method: "POST", body: { source_id: sourceId, category },
  })
  const url = new URL("/static/mobile-intake/index.html", base.origin)
  url.searchParams.set("lang", language.startsWith("zh") ? "zh" : "en")
  url.hash = result.token
  return { url: url.toString(), expiresAt: result.expires_at }
}
