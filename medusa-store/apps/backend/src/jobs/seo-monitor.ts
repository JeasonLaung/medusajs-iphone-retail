import type { MedusaContainer } from "@medusajs/framework/types"
import { seoCommandWorkflow } from "../workflows/seo-command"
export default async function seoMonitor(container: MedusaContainer) {
  await seoCommandWorkflow(container).run({ input: { action: "tick" } })
}
export const config = { name: "seo-monitor", schedule: "* * * * *" }
