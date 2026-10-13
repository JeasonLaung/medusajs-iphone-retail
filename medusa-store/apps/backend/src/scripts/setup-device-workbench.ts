import type { ExecArgs } from "@medusajs/framework/types"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"
import { createDeviceSourceWorkflow } from "../workflows/device-source"

export default async function setupDeviceWorkbench({ container }: ExecArgs) {
  const service: DevicesModuleService = container.resolve(DEVICES_MODULE)
  const sources = await service.listDeviceSources({})
  for (const type of ["self", "supplier"] as const) {
    if (sources.some((s) => s.type === type && !s.name.startsWith("Check ")))
      continue
    await createDeviceSourceWorkflow(container).run({
      input: { type, name: type === "self" ? "自有仓库" : "供应商代发" },
    })
  }
  console.log(
    "Device sources ready. Enroll the selected SKU in the workbench, then import actual devices or supplier quantities.",
  )
}
