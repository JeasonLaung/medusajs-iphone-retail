import path from "node:path"
import fs from "node:fs"
import { MedusaError } from "@medusajs/framework/utils"
import type { DepOptimizationOptions, Plugin } from "vite" with { "resolution-mode": "import" }

export function transformWarehouseMenu(source: string, id: string) {
  if (!/\/@medusajs\/dashboard\/dist\/chunk-[^/]+\.mjs$/.test(id.replace(/\\/g, "/").split("?")[0]) || !source.includes("var SidebarRoutes = () => {")) return
  const anchor = 'var SidebarRoutes = () => {\n  const coreRoutes = useCoreRoutes();\n  const { getMenu } = useExtension();\n  const menuItems = getMenu("coreExtensions");'
  if (!source.includes(anchor)) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Warehouse menu: dashboard sidebar integration changed")
  const utilityAnchor = "var UtilitySection = () => {\n  const location = useLocation6();\n  const { t: t2 } = useTranslation10();"
  if (!source.includes(utilityAnchor)) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "Warehouse menu: dashboard utility integration changed")
  const component = path.resolve(__dirname, "../admin/lib/use-warehouse-menu.ts").replace(/\\/g, "/")
  // Keep every hook unconditional while the access query loads or refreshes.
  return `import { useWarehouseMenu, useWarehouseAccess } from ${JSON.stringify(component)}\n${source.replace(anchor, 'var SidebarRoutes = () => {\n  const filterWarehouseMenu = useWarehouseMenu();\n  const coreRoutes = filterWarehouseMenu(useCoreRoutes());\n  const { getMenu } = useExtension();\n  const menuItems = filterWarehouseMenu(getMenu("coreExtensions"));').replace(utilityAnchor, `${utilityAnchor}\n  const { data: warehouseAccess } = useWarehouseAccess();\n  if (!warehouseAccess || warehouseAccess.warehouse) return null;`)}`
}
export const adminWarehouseMenu = (): Plugin => ({ name: "admin-warehouse-menu", enforce: "pre", transform(source, id) {
  const code = transformWarehouseMenu(source, id)
  if (code) return { code, map: null }
} })
type OptimizationPlugin = NonNullable<NonNullable<DepOptimizationOptions["esbuildOptions"]>["plugins"]>[number]
export const adminWarehouseMenuOptimizer = (): OptimizationPlugin => ({ name: "admin-warehouse-menu-optimizer-v2", setup(build) {
  build.onResolve({ filter: /\/src\/admin\/lib\/use-warehouse-menu\.ts$/ }, (args) => ({ path: `/@fs/${args.path}`, external: true }))
  build.onLoad({ filter: /chunk-[^/\\]+\.mjs$/ }, (args) => {
    const contents = transformWarehouseMenu(fs.readFileSync(args.path, "utf8"), args.path)
    if (contents) return { contents, loader: "js", resolveDir: path.dirname(args.path) }
  })
} })
