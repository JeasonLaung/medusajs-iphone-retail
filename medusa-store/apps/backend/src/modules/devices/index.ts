import { Module } from "@medusajs/framework/utils"
import DevicesModuleService from "./service"

export const DEVICES_MODULE = "devices"
export default Module(DEVICES_MODULE, { service: DevicesModuleService })
