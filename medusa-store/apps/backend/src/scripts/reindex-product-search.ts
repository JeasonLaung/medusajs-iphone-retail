import type { ExecArgs, ISearchModuleService } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"

export default async function reindexProductSearch({ container }: ExecArgs) {
  const service: ISearchModuleService = container.resolve(Modules.SEARCH)
  const result = await service.reindex({ index: "product", strategy: "swap" })
  console.log(`Product search rebuilt: ${result.job_id}`)
}
