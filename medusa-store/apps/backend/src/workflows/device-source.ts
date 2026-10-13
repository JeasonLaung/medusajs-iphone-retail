import {
  createStep,
  createWorkflow,
  StepResponse,
  WorkflowResponse,
  transform,
} from "@medusajs/framework/workflows-sdk"
import {
  ContainerRegistrationKeys,
  Modules,
  MedusaError,
} from "@medusajs/framework/utils"
import {
  acquireLockStep,
  releaseLockStep,
  createStockLocationsWorkflow,
  linkSalesChannelsToStockLocationWorkflow,
  createRemoteLinkStep,
} from "@medusajs/medusa/core-flows"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"

const sourceContext = createStep(
  "source-context",
  async (input: { name: string; type: "self" | "supplier" }, { container }) => {
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    if ((await service.listDeviceSources({ name: input.name })).length)
      throw new MedusaError(MedusaError.Types.INVALID_DATA, "该来源名称已存在")
    const query = container.resolve(ContainerRegistrationKeys.QUERY)
    const { data } = await query.graph({
      entity: "store",
      fields: ["default_sales_channel_id"],
    })
    if (!data[0]?.default_sales_channel_id)
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        "请先配置默认销售渠道",
      )
    return new StepResponse({ channel_id: data[0].default_sales_channel_id })
  },
)
const saveSource = createStep(
  "save-source",
  async (
    input: { name: string; type: "self" | "supplier"; location_id: string },
    { container },
  ) => {
    const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
    const source = await service.createDeviceSources(input)
    return new StepResponse(source, source.id)
  },
  async (id, { container }) => {
    if (id)
      await container
        .resolve<DevicesModuleService>(DEVICES_MODULE)
        .deleteDeviceSources(id)
  },
)

export const createDeviceSourceWorkflow = createWorkflow(
  "create-device-source",
  (input: { name: string; type: "self" | "supplier" }) => {
    acquireLockStep({ key: "device-operations", timeout: 30, ttl: 180 })
    const context = sourceContext(input)
    const locations = createStockLocationsWorkflow.runAsStep({
      input: { locations: [{ name: input.name }] },
    })
    const location = transform(locations, (locations) => locations[0])
    linkSalesChannelsToStockLocationWorkflow.runAsStep({
      input: { id: location.id, add: [context.channel_id] },
    })
    createRemoteLinkStep([
      {
        [Modules.STOCK_LOCATION]: { stock_location_id: location.id },
        [Modules.FULFILLMENT]: { fulfillment_provider_id: "manual_manual" },
      },
    ])
    const source = saveSource({
      name: input.name,
      type: input.type,
      location_id: location.id,
    })
    releaseLockStep({ key: "device-operations" })
    return new WorkflowResponse(source)
  },
)
