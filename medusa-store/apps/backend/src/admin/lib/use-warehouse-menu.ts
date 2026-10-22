import { useQuery } from "@tanstack/react-query"
import { sdk } from "./sdk"

type MenuItem = { to: string; items?: MenuItem[] }
export type Access = { warehouse: boolean; permissions: string[] }
export function warehouseMenu<T extends MenuItem>(items: T[], access?: Access): T[] {
  if (!access) return []
  if (!access.warehouse) return items
  const permitted: Record<string, boolean> = {
    "/orders": access.permissions.includes("order:read"),
    "/device-scan": access.permissions.includes("warehouse_intake:read") || access.permissions.includes("warehouse_return:read"),
    "/device-scan/intake": access.permissions.includes("warehouse_intake:read"),
    "/device-scan/returned": access.permissions.includes("warehouse_return:read"),
  }
  return items.filter((item) => permitted[item.to]).map((item) => ({ ...item, items: item.items ? warehouseMenu(item.items, access) : undefined }))
}
export function useWarehouseAccess() {
  return useQuery({ queryKey: ["warehouse-menu"], queryFn: () => sdk.client.fetch<Access>("/admin/warehouse-access/me"), staleTime: 0, refetchInterval: 30_000 })
}
export function useWarehouseMenu() {
  const { data } = useWarehouseAccess()
  return <T extends MenuItem>(items: T[]) => warehouseMenu(items, data)
}
