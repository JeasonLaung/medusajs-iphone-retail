import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
} from "@medusajs/framework/workflows-sdk"
import { MedusaError, Modules } from "@medusajs/framework/utils"
import type { IUserModuleService } from "@medusajs/framework/types"
import { updateUsersWorkflow } from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { deviceAccess, supplierDenied } from "../api/device-access"

type Input = {
  actor_id: string
  email: string
  source_id: string
  order_ids: string[]
}
const prepareAccount = createStep(
  "prepare-account",
  async (input: Input, { container }) => {
    if ((await deviceAccess(container, input.actor_id)).role !== "admin")
      throw supplierDenied()
    const users: IUserModuleService = container.resolve(Modules.USER)
    const [user] = await users.listUsers({ email: input.email })
    if (!user)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Supplier account not found",
      )
    if (user.id === input.actor_id)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Cannot bind your own account as supplier",
      )
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const [source] = await service.listDeviceSources({
      id: input.source_id,
      type: "supplier",
    })
    if (!source)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Select a supplier source",
      )
    const orders = container.resolve(Modules.ORDER)
    for (const id of input.order_ids) await orders.retrieveOrder(id)
    return new StepResponse({
      id: user.id,
      metadata: {
        ...user.metadata,
        device_role: "supplier",
        device_source_id: source.id,
        device_order_ids: [...new Set(input.order_ids)],
      },
    })
  },
)

export const deviceSupplierAccountWorkflow = createWorkflow(
  "device-supplier-account",
  (input: Input) => {
    const update = prepareAccount(input)
    const result = updateUsersWorkflow.runAsStep({
      input: { updates: [update] },
    })
    return new WorkflowResponse(result)
  },
)

const createSupplierAccount = createStep(
  "create-supplier-account",
  async (input: Input & { password: string }, { container }) => {
    if ((await deviceAccess(container, input.actor_id)).role !== "admin")
      throw supplierDenied()
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const [source] = await service.listDeviceSources({
      id: input.source_id,
      type: "supplier",
    })
    if (!source)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Select a supplier source",
      )
    const users: IUserModuleService = container.resolve(Modules.USER)
    if ((await users.listUsers({ email: input.email })).length)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Supplier account already exists; leave password empty to update permissions",
      )
    for (const id of input.order_ids)
      await container.resolve(Modules.ORDER).retrieveOrder(id)
    const auth = container.resolve(Modules.AUTH)
    const registration = await auth.register("emailpass", {
      body: { email: input.email, password: input.password },
    })
    if (!registration.success || !registration.authIdentity)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "Unable to create supplier login",
      )
    const authId = registration.authIdentity.id
    let userId: string | undefined
    try {
      // Assign restrictions before linking the login identity to this user.
      const user = await users.createUsers({
        email: input.email,
        metadata: {
          device_role: "supplier",
          device_source_id: source.id,
          device_order_ids: [...new Set(input.order_ids)],
        },
      })
      userId = user.id
      await auth.updateAuthIdentities({
        id: authId,
        app_metadata: { user_id: user.id },
      })
      return new StepResponse({ id: user.id }, { userId: user.id, authId })
    } catch (error) {
      if (userId) await users.deleteUsers([userId])
      await auth.deleteAuthIdentities([authId])
      throw error
    }
  },
  async (saved, { container }) => {
    if (!saved) return
    await container.resolve(Modules.AUTH).deleteAuthIdentities([saved.authId])
    await container.resolve(Modules.USER).deleteUsers([saved.userId])
  },
)

export const createDeviceSupplierAccountWorkflow = createWorkflow(
  "create-device-supplier-account",
  (input: Input & { password: string }) => {
    return new WorkflowResponse(createSupplierAccount(input))
  },
)
