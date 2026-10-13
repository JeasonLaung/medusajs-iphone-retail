import { MedusaService } from "@medusajs/framework/utils"
import {
  Device,
  DeviceSource,
  DeviceVariant,
  DeviceSupply,
  DeviceAssignment,
  DeviceEvent,
} from "./models/device"

export default class DevicesModuleService extends MedusaService({
  Device,
  DeviceSource,
  DeviceVariant,
  DeviceSupply,
  DeviceAssignment,
  DeviceEvent,
}) {}
