import path from "node:path"
import fs from "node:fs"
import { MedusaError } from "@medusajs/framework/utils"
import type { DepOptimizationOptions, Plugin } from "vite" with { "resolution-mode": "import" }

// Extend the built-in list without maintaining a copy or editing node_modules.
// Fail explicitly if a dashboard upgrade changes the two header definitions.
const transformInventoryColumns = (source: string, id: string) => {
  const filename = id.replace(/\\/g, "/").split("?")[0]
  const sourceModule = filename.endsWith(
    "/routes/inventory/inventory-list/components/use-inventory-table-columns.tsx"
  )
  const bundledModule = /\/@medusajs\/dashboard\/dist\/inventory-list-[^/]+\.mjs$/.test(filename)
  if (!sourceModule && !bundledModule) return

  const component = path.resolve(__dirname, "../admin/components/inventory-sort-header.tsx").replace(/\\/g, "/")
  let code = source
  for (const [field, key] of [
    ["reserved_quantity", "inventory.reserved"],
    ["stocked_quantity", "fields.inStock"],
  ]) {
    const original = `header: t("${key}"),`
    if (!code.includes(original)) {
      throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `Inventory sorting: dashboard header changed for ${field}`)
    }
    const header = sourceModule
      ? `<InventorySortHeader field="${field}" />`
      : `inventorySortJsx(InventorySortHeader, { field: "${field}" })`
    code = code.replace(original, `header: () => ${header},`)
  }
  return `import { jsx as inventorySortJsx } from "react/jsx-runtime"\nimport { InventorySortHeader } from ${JSON.stringify(component)}\n${code}`
}

export const adminInventorySorting = (): Plugin => ({
  name: "admin-inventory-sorting",
  enforce: "pre",
  transform(source, id) {
    const code = transformInventoryColumns(source, id)
    if (code) return { code, map: null }
  },
})

type OptimizationPlugin = NonNullable<NonNullable<DepOptimizationOptions["esbuildOptions"]>["plugins"]>[number]

// Development prebundles the dashboard before Vite's transform hooks run.
export const adminInventorySortingOptimizer = (): OptimizationPlugin => ({
  name: "admin-inventory-sorting-optimizer",
  setup(build) {
    build.onLoad({ filter: /inventory-list-[^/\\]+\.mjs$/ }, (args) => {
      const contents = transformInventoryColumns(fs.readFileSync(args.path, "utf8"), args.path)
      if (contents) return { contents, loader: "js", resolveDir: path.dirname(args.path) }
    })
  },
})
