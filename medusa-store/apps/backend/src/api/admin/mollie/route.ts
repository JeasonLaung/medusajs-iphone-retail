import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { MedusaError } from "@medusajs/framework/utils"
import { MOLLIE_SETTINGS_MODULE } from "../../../modules/mollie-settings"
import type MollieSettingsModuleService from "../../../modules/mollie-settings/service"
import { decryptMollieSecret, encryptMollieSecret } from "../../../modules/mollie-settings/security"
import { saveMollieConfigWorkflow, type MollieConfigInput } from "../../../workflows/save-mollie-config"

type MollieBody = { environment?: "test" | "production"; api_key?: string; redirect_url?: string; enabled?: boolean }

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service: MollieSettingsModuleService = req.scope.resolve(MOLLIE_SETTINGS_MODULE)
  const [configs] = await service.listAndCountMollieConfigs({}, { take: 1 })
  const config = configs[0]
  return res.json({
    config: config
      ? { environment: config.environment, redirect_url: config.redirect_url, enabled: config.enabled, has_api_key: !!config.api_key, api_key_hint: config.api_key ? `••••${decryptMollieSecret(config.api_key).slice(-4)}` : "" }
      : { environment: "test", redirect_url: process.env.MOLLIE_REDIRECT_URL || `${process.env.STOREFRONT_URL || "http://localhost:8000"}/api/mollie-return`, enabled: false, has_api_key: !!process.env.MOLLIE_API_KEY, api_key_hint: process.env.MOLLIE_API_KEY ? `••••${process.env.MOLLIE_API_KEY.slice(-4)}` : "" },
  })
}

export async function POST(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const body = (req.body || {}) as MollieBody
  if (!body.environment || !["test", "production"].includes(body.environment)) throw new MedusaError(MedusaError.Types.INVALID_DATA, "Choose test or production mode")
  const data: Record<string, unknown> = {
    environment: body.environment,
    redirect_url: body.redirect_url || `${process.env.STOREFRONT_URL || "http://localhost:8000"}/api/mollie-return`,
    enabled: body.enabled !== false,
  }
  if (body.api_key?.trim()) {
    const expected = body.environment === "test" ? "test_" : "live_"
    if (!body.api_key.startsWith(expected)) throw new MedusaError(MedusaError.Types.INVALID_DATA, `A ${body.environment} Mollie key must start with ${expected}`)
    data.api_key = encryptMollieSecret(body.api_key.trim())
  }
  const { result: saved } = await saveMollieConfigWorkflow(req.scope).run({ input: data as MollieConfigInput })
  return res.json({ config: { environment: saved.environment, redirect_url: saved.redirect_url, enabled: saved.enabled, has_api_key: !!saved.api_key } })
}
