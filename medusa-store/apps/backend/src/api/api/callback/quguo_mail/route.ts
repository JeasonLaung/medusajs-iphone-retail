import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { receiveRelayme } from "../../../relayme-callback"

export async function POST(req: MedusaRequest, res: MedusaResponse) {
  return receiveRelayme(req, res, "inbound")
}
