import type { MedusaContainer } from "@medusajs/framework/types"
import { expireBuybackPhotosWorkflow } from "../workflows/expire-buyback-photos"

export default async function expireBuybackPhotos(container: MedusaContainer) {
  await expireBuybackPhotosWorkflow(container).run({ input: {} })
}
export const config = { name: "expire-buyback-photos", schedule: "0 3 * * *" }
