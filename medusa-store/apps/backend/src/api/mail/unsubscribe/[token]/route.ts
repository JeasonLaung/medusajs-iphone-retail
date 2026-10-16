import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { unsubscribeMailWorkflow } from "../../../../workflows/unsubscribe-mail"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  if (!/^[a-f0-9]{64}$/.test(req.params.token)) return res.sendStatus(404)
  res.setHeader(
    "Content-Security-Policy",
    "default-src 'none'; form-action 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'"
  )
  res.setHeader("Cache-Control", "no-store")
  res
    .type("html")
    .send(
      '<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Unsubscribe / 退订</title><body style="font:16px system-ui;max-width:480px;margin:15vh auto;padding:24px"><h1>Unsubscribe / 取消订阅</h1><p>Stop receiving marketing emails.<br>停止接收营销邮件。</p><form method="post"><button style="padding:12px 24px">Confirm unsubscribe / 确认退订</button></form></body></html>'
    )
}

export async function POST(req: MedusaRequest, res: MedusaResponse) {
  if (!/^[a-f0-9]{64}$/.test(req.params.token)) return res.sendStatus(404)
  const { result } = await unsubscribeMailWorkflow(req.scope).run({
    input: { token: req.params.token },
  })
  res.setHeader("Cache-Control", "no-store")
  res
    .status(result.ok ? 200 : 404)
    .type("text")
    .send(
      result.ok ? "Unsubscribed. 您已取消订阅。" : "Invalid link. 链接无效。"
    )
}
