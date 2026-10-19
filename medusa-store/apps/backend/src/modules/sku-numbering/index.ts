import { MedusaService, Module } from "@medusajs/framework/utils"

class SkuNumberingModuleService extends MedusaService({}) {}

export default Module("skuNumbering", {
  service: SkuNumberingModuleService,
})
