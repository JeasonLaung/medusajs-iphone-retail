import { Module } from "@medusajs/framework/utils"
import AfterSalesModuleService from "./service"

export const AFTER_SALES_MODULE = "after_sales"

export default Module(AFTER_SALES_MODULE, { service: AfterSalesModuleService })
