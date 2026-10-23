import { Module } from "@medusajs/framework/utils"
import BuybackModuleService from "./service"

export const BUYBACK_MODULE = "buyback"
export default Module(BUYBACK_MODULE, { service: BuybackModuleService })
