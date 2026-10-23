import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { CUSTOMER_NOTIFICATIONS_MODULE } from "../modules/customer-notifications"
import type CustomerNotificationsService from "../modules/customer-notifications/service"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"

export type NoticeInput = { customer_id: string; event_key: string; kind: string; resource_id: string; reference: string; action?: string }
type NoticeRollback = { id: string; previous?: { event_key: string; action: string; reference: string } }
export const createCustomerNoticeStep = createStep("create-customer-notice", async (input: NoticeInput | null, { container }) => {
  if (!input?.customer_id) return new StepResponse(null)
  const service = container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  const [existing] = await service.listCustomerNotices({ event_key: input.event_key })
  if (existing) return new StepResponse(existing, null)
  // Buyback commands already hold the request lock. Keep one unread update per request.
  if (input.kind === "buyback.updated") {
    const [unread] = await service.listCustomerNotices({ customer_id: input.customer_id, kind: input.kind, resource_id: input.resource_id, read_at: null }, { take: 1, order: { created_at: "DESC" } })
    if (unread) {
      const [notice] = await service.updateCustomerNotices({ selector: { id: unread.id, read_at: null }, data: { event_key: input.event_key, action: input.action || "", reference: input.reference } })
      // A simultaneous read must not swallow the new reminder.
      if (notice) return new StepResponse(notice, { id: unread.id, previous: { event_key: unread.event_key, action: unread.action, reference: unread.reference } })
    }
  }
  const notice = await service.createCustomerNotices(input)
  return new StepResponse(notice, { id: notice.id })
}, async (rollback: NoticeRollback | null | undefined, { container }) => {
  if (!rollback) return
  const service = container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  if (rollback.previous) await service.updateCustomerNotices({ id: rollback.id, ...rollback.previous })
  else await service.deleteCustomerNotices(rollback.id)
})

export const createCustomerNoticeWorkflow = createWorkflow("create-customer-notice", function (input: NoticeInput) {
  acquireLockStep({ key: input.event_key, timeout: 10, ttl: 60 })
  const notice = createCustomerNoticeStep(input)
  releaseLockStep({ key: input.event_key })
  return new WorkflowResponse(notice)
})

const readNoticesStep = createStep("read-notices", async (input: { customer_id: string; id?: string }, { container }) => {
  const service = container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  // Apply the ownership and unread filters to the mutation itself, including mark-all.
  await service.updateCustomerNotices({ selector: { customer_id: input.customer_id, read_at: null, ...(input.id ? { id: input.id } : {}) }, data: { read_at: new Date() } })
  return new StepResponse({ success: true })
})
export const readCustomerNoticesWorkflow = createWorkflow("read-customer-notices", function (input: { customer_id: string; id?: string }) {
  return new WorkflowResponse(readNoticesStep(input))
})
