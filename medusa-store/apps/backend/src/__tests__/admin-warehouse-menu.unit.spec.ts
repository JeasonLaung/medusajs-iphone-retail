import fs from "node:fs"
import path from "node:path"
import vm from "node:vm"
import { transformWarehouseMenu } from "../lib/admin-warehouse-menu"

const dist = path.join(path.dirname(require.resolve("@medusajs/dashboard/package.json")), "dist")
const filename = fs.readdirSync(dist).find((name) => name.startsWith("chunk-") && name.endsWith(".mjs") && fs.readFileSync(path.join(dist, name), "utf8").includes("var SidebarRoutes = () => {"))!
const id = path.join(dist, filename)
const source = fs.readFileSync(id, "utf8")

test("settings calls the same hooks before and after warehouse access resolves", () => {
  const transformed = transformWarehouseMenu(source, id)!
  expect(() => require("@swc/core").parseSync(transformed, { syntax: "ecmascript" })).not.toThrow()
  const utility = transformed.slice(transformed.indexOf("var UtilitySection = () => {"), transformed.indexOf("var UserSection = () => {"))
  const calls: string[] = []
  let access: { warehouse: boolean } | undefined
  const context = vm.createContext({
    useLocation6: () => { calls.push("location"); return { pathname: "/orders" } },
    useTranslation10: () => { calls.push("translation"); return { t: (key: string) => key } },
    useWarehouseAccess: () => { calls.push("access"); return { data: access } },
    jsx16: (type: unknown, props: unknown) => ({ type, props }),
    NavItem: "NavItem",
    CogSixTooth: "CogSixTooth",
  })
  vm.runInContext(utility, context)
  for (access of [undefined, { warehouse: false }, { warehouse: true }, undefined, { warehouse: false }]) {
    calls.length = 0
    const result = vm.runInContext("UtilitySection()", context)
    expect(calls).toEqual(["location", "translation", "access"])
    if (!access || access.warehouse) expect(result).toBeNull()
    else expect(result.props.children.props.to).toBe("/settings")
  }
})

test("fails explicitly if the dashboard utility hook integration changes", () => {
  expect(() => transformWarehouseMenu(source.replace("var UtilitySection = () => {", "var ChangedUtility = () => {"), id)).toThrow("dashboard utility integration changed")
})
