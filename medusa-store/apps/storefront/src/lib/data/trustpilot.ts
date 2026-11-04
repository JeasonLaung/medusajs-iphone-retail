import "server-only"
import { sdk } from "@lib/config"
import type { TrustpilotSettings } from "types/trustpilot"

export async function getTrustpilotSettings(): Promise<TrustpilotSettings | null> {
  try {
    const { settings } = await sdk.client.fetch<{
      settings: TrustpilotSettings | null
    }>("/store/trustpilot", { cache: "no-store" })
    return settings?.enabled ? settings : null
  } catch {
    return null
  }
}
