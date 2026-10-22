import { definePolicies } from "@medusajs/framework/utils"

export const warehousePolicies = definePolicies([
  { resource: "warehouse_intake", operation: "read", name: "View warehouse intake" },
  { resource: "warehouse_intake", operation: "update", name: "Manage warehouse intake" },
  { resource: "warehouse_return", operation: "read", name: "View warehouse returns" },
  { resource: "warehouse_return", operation: "update", name: "Manage warehouse returns" },
  { resource: "warehouse_allocation", operation: "update", name: "Allocate warehouse devices" },
])
