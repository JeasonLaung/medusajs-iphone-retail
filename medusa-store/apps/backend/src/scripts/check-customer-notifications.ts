import assert from "node:assert/strict"
import { randomUUID } from "node:crypto"
import type { ExecArgs, ConfigModule } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, generateJwtToken, Modules } from "@medusajs/framework/utils"
import { CUSTOMER_NOTIFICATIONS_MODULE } from "../modules/customer-notifications"
import type CustomerNotificationsService from "../modules/customer-notifications/service"
import { createCustomerNoticeWorkflow, readCustomerNoticesWorkflow } from "../workflows/customer-notifications"
import { GET } from "../api/store/notifications/route"

export default async function checkCustomerNotifications({ container }: ExecArgs) {
  const service = container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  const suffix = randomUUID()
  const owner = `notice_test_${suffix}`
  const other = `notice_other_${suffix}`
  async function list(customer_id: string) {
    let response: any
    await GET({ scope: container, query: {}, auth_context: { actor_id: customer_id } } as any, { json: (value: unknown) => { response = value } } as any)
    return response
  }
  try {
    const input = { customer_id: owner, event_key: `test:${suffix}`, kind: "shipment.created", resource_id: "order_test", reference: "TEST" }
    const first = await createCustomerNoticeWorkflow(container).run({ input })
    const duplicate = await createCustomerNoticeWorkflow(container).run({ input })
    assert.equal(first.result?.id, duplicate.result?.id)
    assert.equal((await list(owner)).count, 1)
    assert.equal((await list(owner)).unread, 1)
    assert.equal((await list(other)).count, 0)
    await readCustomerNoticesWorkflow(container).run({ input: { customer_id: other, id: first.result!.id } })
    assert.equal((await list(owner)).unread, 1)
    await readCustomerNoticesWorkflow(container).run({ input: { customer_id: owner, id: first.result!.id } })
    assert.equal((await list(owner)).unread, 0)
    await createCustomerNoticeWorkflow(container).run({ input: { ...input, event_key: `other:${suffix}` } })
    await readCustomerNoticesWorkflow(container).run({ input: { customer_id: owner } })
    assert.equal((await list(owner)).unread, 0)
    assert.equal((await list(owner)).count, 2)
    const config = container.resolve<ConfigModule>(ContainerRegistrationKeys.CONFIG_MODULE)
    const keys = await container.resolve(Modules.API_KEY).listApiKeys({ type: "publishable" })
    const token = generateJwtToken({ actor_id: owner, actor_type: "customer", auth_identity_id: `test_${suffix}`, app_metadata: { customer_id: owner } }, { secret: config.projectConfig.http.jwtSecret!, expiresIn: "60s" })
    const base = "http://127.0.0.1:9001/store/notifications"
    const headers = { "x-publishable-api-key": keys[0].token }
    assert.equal((await fetch(base, { headers })).status, 401)
    const response = await fetch(base, { headers: { ...headers, authorization: `Bearer ${token}` } })
    assert.equal(response.status, 200)
    const page = await response.json()
    assert.equal(page.count, 2)
    assert.equal(page.unread, 0)
    console.log("PASS notification persistence, duplicate suppression, customer isolation, single/all read and authenticated HTTP")
  } finally {
    await service.deleteCustomerNotices({ customer_id: owner })
    await service.deleteCustomerNotices({ customer_id: other })
  }
}
