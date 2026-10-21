import { Module } from "@medusajs/framework/utils"
import MollieSettingsModuleService from "./service"

export const MOLLIE_SETTINGS_MODULE = "mollieSettings"
export default Module(MOLLIE_SETTINGS_MODULE, { service: MollieSettingsModuleService })
