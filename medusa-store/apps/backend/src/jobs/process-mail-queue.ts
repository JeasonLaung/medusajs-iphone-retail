import type { MedusaContainer } from "@medusajs/framework/types"
import { processMailQueueWorkflow } from "../workflows/process-mail-queue"

export default async function processMailQueue(container: MedusaContainer) {
  await processMailQueueWorkflow(container).run({ input: {} })
}

export const config = { name: "process-mail-queue", schedule: "* * * * *" }
