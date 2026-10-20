import { MedusaService } from "@medusajs/framework/utils"
import {
  Device,
  DeviceSource,
  DeviceVariant,
  DeviceSupply,
  DeviceAssignment,
  DeviceEvent,
} from "./models/device"

const BaseDevicesModuleService = MedusaService({
  Device,
  DeviceSource,
  DeviceVariant,
  DeviceSupply,
  DeviceAssignment,
  DeviceEvent,
})

export default class DevicesModuleService extends BaseDevicesModuleService {
  // Device records must survive every deletion path, including legacy callers.
  deleteDevices = async (...args: Parameters<InstanceType<typeof BaseDevicesModuleService>["deleteDevices"]>): Promise<void> => {
    const [ids, context] = args
    await this.softDeleteDevices(ids, {}, context)
  }
}
