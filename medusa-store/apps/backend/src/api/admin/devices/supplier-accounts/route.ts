import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import type { IUserModuleService } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { z } from "@medusajs/framework/zod"
import { deviceAccess, supplierDenied } from "../../../device-access"
import {
  deviceSupplierAccountWorkflow,
  createDeviceSupplierAccountWorkflow,
} from "../../../../workflows/device-supplier-account"

export const SupplierAccountBody = z
  .object({
    email: z.string().trim().email().max(254),
    source_id: z.string().trim().min(1).max(100),
    order_ids: z.array(z.string().trim().min(1).max(100)).max(200),
    password: z.string().min(12).max(128).optional(),
  })
  .strict()

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse,
) {
  if (
    (await deviceAccess(req.scope, req.auth_context.actor_id)).role !== "admin"
  )
    throw supplierDenied()
  const users: IUserModuleService = req.scope.resolve(Modules.USER)
  const accounts = await users.listUsers({}, { take: 1000 })
  res.json({
    accounts: accounts
      .filter((user) => user.metadata?.device_role === "supplier")
      .map((user) => ({
        email: user.email,
        source_id: user.metadata?.device_source_id,
        order_ids: user.metadata?.device_order_ids || [],
      })),
  })
}

export async function POST(
  req: AuthenticatedMedusaRequest<z.infer<typeof SupplierAccountBody>>,
  res: MedusaResponse,
) {
  if (req.validatedBody.password) {
    await createDeviceSupplierAccountWorkflow(req.scope).run({
      input: {
        ...req.validatedBody,
        password: req.validatedBody.password,
        actor_id: req.auth_context.actor_id,
      },
    })
    res.status(201).json({ success: true })
    return
  }
  await deviceSupplierAccountWorkflow(req.scope).run({
    input: { ...req.validatedBody, actor_id: req.auth_context.actor_id },
  })
  res.json({ success: true })
}
