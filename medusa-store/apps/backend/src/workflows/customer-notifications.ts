import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { CUSTOMER_NOTIFICATIONS_MODULE } from "../modules/customer-notifications"
import type CustomerNotificationsService from "../modules/customer-notifications/service"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"

export type NoticeInput = { customer_id: string; event_key: string; kind: string; resource_id: string; reference: string; action?: string }
export const createCustomerNoticeStep = createStep("create-customer-notice", async (input: NoticeInput | null, { container }) => {
  if (!input?.customer_id) return new StepResponse(null)
  const service = container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE)
  const [existing] = await service.listCustomerNotices({ event_key: input.event_key })
  if (existing) return new StepResponse(existing, null)
  const notice = await service.createCustomerNotices(input)
  return new StepResponse(notice, notice.id)
}, async (id: string | null | undefined, { container }) => {
  if (id) await container.resolve<CustomerNotificationsService>(CUSTOMER_NOTIFICATIONS_MODULE).deleteCustomerNotices(id)
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
