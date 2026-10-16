import { MedusaService, Module } from "@medusajs/framework/utils"

// This module owns the database allocator and order insert trigger. Keeping the
// allocation in the insert transaction covers every Medusa order creation path.
class OrderNumberingModuleService extends MedusaService({}) {}

export default Module("orderNumbering", {
  service: OrderNumberingModuleService,
})
