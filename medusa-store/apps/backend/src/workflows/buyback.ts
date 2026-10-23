import { randomUUID } from "node:crypto"
import { createCustomerNoticeStep } from "./customer-notifications"
import { createStep, createWorkflow, StepResponse, WorkflowResponse, transform } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import type { IFileModuleService } from "@medusajs/framework/types"
import { acquireLockStep, releaseLockStep } from "@medusajs/medusa/core-flows"
import { BUYBACK_MODULE } from "../modules/buyback"
import type BuybackModuleService from "../modules/buyback/service"
import { adminBuybackAccess, assertBuyback, assertPhotos, customerBuyback, latestPhotos, publicPhoto, requestItems, serializeBuyback, type BuybackItem, type RequestRecord, type PhotoRecord } from "../lib/buyback"
import type { BuybackAdminCommand, BuybackCustomerCommand, BuybackPhotoUpload, BuybackReferencePrice, CreateBuyback } from "../api/buyback-validators"
import { assessmentPrice, normalizeCapacity, type PricingRules } from "../lib/buyback-pricing"

const json = (value: unknown) => value as Record<string, unknown>
const amount = (values: number[]) => Math.round(values.reduce((sum, value) => sum + value, 0) * 100) / 100
const normalize = (value: string) => value.replace(/\s+/g, "").toUpperCase()

type CreateInput = CreateBuyback & { customer_id: string }
const createBuybackRequest = createStep("create-buyback-request", async (input: CreateInput, { container }) => {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const [existing] = await service.listBuybackRequests({ customer_id: input.customer_id, submission_key: input.submission_key })
  if (existing) return new StepResponse(await serializeBuyback(container, existing), "")
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: customers } = await query.graph({ entity: "customer", fields: ["id", "email"], filters: { id: input.customer_id } })
  assertBuyback(customers[0]?.email, "BUYBACK_DENIED")
  const items: BuybackItem[] = []
  for (const item of input.items) {
    const { data: products } = await query.graph({ entity: "product", fields: ["id", "title", "handle", "options.title", "options.values.value"], filters: { handle: [item.model, `swappie-${item.model}`], status: "published" } })
    const product = products.find((product) => /^iPhone\b/i.test(product.title))
    assertBuyback(product, "BUYBACK_INVALID_MODEL")
    assertBuyback(product?.options?.some((option) => /storage|capacity|memory|容量/i.test(option.title) && option.values?.some((value) => normalizeCapacity(value.value) === normalizeCapacity(item.capacity))), "BUYBACK_INVALID_MODEL")
    const [price] = await service.listBuybackPrices({ product_id: product.id, capacity: normalizeCapacity(item.capacity) })
    items.push({ id: `bbi_${randomUUID().replace(/-/g, "")}`, product_id: product.id, model: item.model, model_name: product.title, capacity: item.capacity, assessment: item.assessment,
      reference_amount: assessmentPrice(price?.pricing_rules as PricingRules | null, item.assessment, price ? Number(price.amount) : null).amount, final_amount: null, inspection_notes: "", serial_number: "", imei: "", device_id: null })
  }
  const request = await service.createBuybackRequests({
    customer_id: input.customer_id, submission_key: input.submission_key, email: customers[0].email,
    contact_name: input.contact_name, phone: input.phone, return_address: input.return_address,
    items: json(items), reference_amount: items.every((item) => item.reference_amount != null) ? amount(items.map((item) => item.reference_amount!)) : null,
    history: json([{ at: new Date().toISOString(), action: "submitted", actor: "customer", status: "awaiting_photos" }]),
  })
  return new StepResponse(await serializeBuyback(container, request), request.id)
}, async (id: string | undefined, { container }) => {
  if (id) await container.resolve<BuybackModuleService>(BUYBACK_MODULE).deleteBuybackRequests(id)
})

export const createBuybackWorkflow = createWorkflow("create-buyback", function (input: CreateInput) {
  const key = transform(input, (value) => `buyback-submit:${value.customer_id}:${value.submission_key}`)
  acquireLockStep({ key, timeout: 10, ttl: 60 })
  const result = createBuybackRequest(input)
  releaseLockStep({ key })
  return new WorkflowResponse(result)
})

type CommandInput = { request_id: string } & ({ customer_id: string; command: BuybackCustomerCommand; actor_id?: never } | { actor_id: string; command: BuybackAdminCommand; customer_id?: never })
const snapshot = (request: RequestRecord) => ({ id: request.id, status: request.status, items: request.items, final_amount: request.final_amount, shipping_instructions: request.shipping_instructions, tracking_number: request.tracking_number, return_tracking: request.return_tracking, payment_reference: request.payment_reference, review_note: request.review_note, history: request.history, closed_at: request.closed_at })
type Rollback = { request: ReturnType<typeof snapshot>; photo?: { id: string; review_status: PhotoRecord["review_status"]; review_note: string }; link_id?: string }
const updateBuybackRequest = createStep("update-buyback-request", async (input: CommandInput, { container }) => {
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  if (input.actor_id) await adminBuybackAccess(container, input.actor_id)
  const request = input.customer_id ? await customerBuyback(container, input.request_id, input.customer_id) : await service.retrieveBuybackRequest(input.request_id)
  const before: Rollback = { request: snapshot(request) }
  const items = requestItems(request).map((item) => ({ ...item }))
  let status = request.status
  let change: Partial<ReturnType<typeof snapshot>> = {}
  const photos = await service.listBuybackPhotos({ request_id: request.id }, { take: null })
  const command = input.command
  try {
    if (input.customer_id) {
      const customerCommand = command as BuybackCustomerCommand
      switch (customerCommand.action) {
        case "submit_photos":
          assertBuyback(request.status === "awaiting_photos")
          assertBuyback(customerCommand.items.length === items.length && new Set(customerCommand.items.map((item) => item.item_id)).size === items.length, "BUYBACK_IDENTITY_REQUIRED")
          for (const item of items) {
            const identity = customerCommand.items.find((value) => value.item_id === item.id)
            assertBuyback(identity, "BUYBACK_IDENTITY_REQUIRED")
            item.serial_number = identity.serial_number
            item.imei = identity.imei
          }
          assertBuyback(new Set(items.map((item) => item.serial_number)).size === items.length && new Set(items.map((item) => item.imei)).size === items.length, "BUYBACK_IDENTITY_REQUIRED")
          assertPhotos(items, photos)
          status = "photo_review"
          break
        case "mark_shipped":
          assertBuyback(request.status === "ready_to_ship")
          status = "in_transit"
          change.tracking_number = customerCommand.tracking_number
          break
        case "accept_offer":
          assertBuyback(request.status === "final_offer" && request.final_amount != null && customerCommand.final_amount === Number(request.final_amount), "BUYBACK_OFFER_CHANGED")
          status = "accepted"
          break
        case "decline_offer":
          assertBuyback(request.status === "final_offer")
          status = "return_requested"
          break
        case "cancel":
          assertBuyback(["awaiting_photos", "photo_review", "ready_to_ship"].includes(request.status))
          status = "canceled"
          break
      }
    } else {
      const adminCommand = command as BuybackAdminCommand
      switch (adminCommand.action) {
        case "review_photo": {
          assertBuyback(["awaiting_photos", "photo_review"].includes(request.status))
          const photo = latestPhotos(photos).find((photo) => photo.id === adminCommand.photo_id)
          assertBuyback(photo && photo.review_status !== "expired", "BUYBACK_PHOTO_NOT_FOUND")
          assertBuyback(adminCommand.review_status !== "retake" || adminCommand.note.length >= 3, "BUYBACK_REVIEW_NOTE_REQUIRED")
          before.photo = { id: photo.id, review_status: photo.review_status, review_note: photo.review_note }
          await service.updateBuybackPhotos({ id: photo.id, review_status: adminCommand.review_status, review_note: adminCommand.note })
          if (adminCommand.review_status === "retake") { status = "awaiting_photos"; change.review_note = adminCommand.note }
          break
        }
        case "approve_shipping":
          assertBuyback(request.status === "photo_review")
          assertPhotos(items, photos, true)
          status = "ready_to_ship"
          change.shipping_instructions = adminCommand.shipping_instructions
          break
        case "mark_received":
          assertBuyback(["ready_to_ship", "in_transit"].includes(request.status))
          status = "received"
          break
        case "link_device": {
          assertBuyback(request.status === "received")
          const item = items.find((item) => item.id === adminCommand.item_id)
          assertBuyback(item && !item.device_id, "BUYBACK_DEVICE_MISMATCH")
          const query = container.resolve(ContainerRegistrationKeys.QUERY)
          const { data: devices } = await query.graph({ entity: "device", fields: ["id", "category", "state", "serial_number", "imei", "variant_id"], filters: { id: adminCommand.device_id } })
          const device = devices[0]
          assertBuyback(device?.category === "recycled" && device.serial_number === item.serial_number && device.imei === item.imei && device.state === "inspection", "BUYBACK_DEVICE_MISMATCH")
          const { data: variants } = await query.graph({ entity: "product_variant", fields: ["id", "product_id", "options.value"], filters: { id: device.variant_id } })
          assertBuyback(variants[0]?.product_id === item.product_id && variants[0].options?.some((option) => normalize(option.value) === item.capacity), "BUYBACK_DEVICE_MISMATCH")
          const link = await service.createBuybackDeviceLinks({ request_id: request.id, item_id: item.id, device_id: device.id })
          before.link_id = link.id
          item.device_id = device.id
          break
        }
        case "final_offer":
          assertBuyback(request.status === "received" && items.every((item) => item.device_id), "BUYBACK_LINK_DEVICES_FIRST")
          assertBuyback(adminCommand.items.length === items.length && new Set(adminCommand.items.map((item) => item.item_id)).size === items.length)
          for (const item of items) {
            const offer = adminCommand.items.find((offer) => offer.item_id === item.id)
            assertBuyback(offer)
            item.final_amount = offer.amount
            item.inspection_notes = offer.notes
          }
          status = "final_offer"
          change.final_amount = amount(items.map((item) => item.final_amount!))
          break
        case "mark_paid":
          assertBuyback(request.status === "accepted", "BUYBACK_ACCEPT_FIRST")
          status = "paid"
          change.payment_reference = adminCommand.payment_reference
          break
        case "request_return":
          assertBuyback(["received", "final_offer"].includes(request.status))
          status = "return_requested"
          change.review_note = adminCommand.note
          break
        case "mark_returning":
          assertBuyback(request.status === "return_requested")
          status = "returning"
          change.return_tracking = adminCommand.return_tracking
          break
        case "mark_returned":
          assertBuyback(request.status === "returning")
          status = "returned"
          break
        case "cancel":
          assertBuyback(["awaiting_photos", "photo_review", "ready_to_ship"].includes(request.status))
          status = "canceled"
          change.review_note = adminCommand.note
          break
      }
    }
    const history = Array.isArray(request.history) ? request.history : []
    const updated = await service.updateBuybackRequests({ id: request.id, ...change, items: json(items), status,
      ...(["paid", "returned", "canceled"].includes(status) ? { closed_at: new Date() } : {}),
      history: json([...history, { at: new Date().toISOString(), action: command.action, actor: input.customer_id ? "customer" : "platform", status, note: "note" in command ? command.note : "", ...(command.action === "review_photo" ? { photo_id: command.photo_id } : {}) }]),
    })
    return new StepResponse(await serializeBuyback(container, updated), before)
  } catch (error) {
    if (before.photo) await service.updateBuybackPhotos(before.photo)
    if (before.link_id) await service.deleteBuybackDeviceLinks(before.link_id)
    await service.updateBuybackRequests(before.request)
    throw error
  }
}, async (before: Rollback | undefined, { container }) => {
  if (!before) return
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  if (before.photo) await service.updateBuybackPhotos(before.photo)
  if (before.link_id) await service.deleteBuybackDeviceLinks(before.link_id)
  await service.updateBuybackRequests(before.request)
})
const buybackNoticeInput = createStep("buyback-notice-input", async (input: { request_id: string; actor_id?: string; command: { action: string; review_status?: string } }, { container }) => {
  if (!input.actor_id || input.command.action === "link_device" || (input.command.action === "review_photo" && input.command.review_status !== "retake")) return new StepResponse(null)
  const request = await container.resolve<BuybackModuleService>(BUYBACK_MODULE).retrieveBuybackRequest(input.request_id)
  const history = request.history as unknown as { at: string }[]
  return new StepResponse({ customer_id: request.customer_id, event_key: `buyback:${request.id}:${history[history.length - 1].at}`, kind: "buyback.updated", resource_id: request.id, reference: request.id, action: input.command.action })
})
export const updateBuybackWorkflow = createWorkflow("update-buyback", function (input: CommandInput) {
  acquireLockStep({ key: input.request_id, timeout: 10, ttl: 60 })
  const result = updateBuybackRequest(input)
  const notice = buybackNoticeInput(transform({ input, result }, ({ input, result }) => ({ ...input, request_id: result.id })))
  createCustomerNoticeStep(notice)
  releaseLockStep({ key: input.request_id })
  return new WorkflowResponse(result)
})

type PhotoInput = BuybackPhotoUpload & { request_id: string; customer_id: string }
const uploadBuybackPhoto = createStep("upload-buyback-photo", async (input: PhotoInput, { container }) => {
  const request = await customerBuyback(container, input.request_id, input.customer_id)
  assertBuyback(["awaiting_photos", "ready_to_ship"].includes(request.status))
  assertBuyback(requestItems(request).some((item) => item.id === input.item_id), "BUYBACK_PHOTO_NOT_FOUND")
  assertBuyback(request.status !== "ready_to_ship" || ["packaging", "sealed"].includes(input.photo_type))
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const existing = await service.listBuybackPhotos({ request_id: request.id, item_id: input.item_id }, { take: null })
  assertBuyback(existing.length < 30, "BUYBACK_TOO_MANY_PHOTOS")
  const bytes = Buffer.from(input.content, "base64")
  const mime = bytes.subarray(0, 3).equals(Buffer.from([255, 216, 255])) ? "image/jpeg" : bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ? "image/png" : bytes.toString("ascii", 0, 4) === "RIFF" && bytes.toString("ascii", 8, 12) === "WEBP" ? "image/webp" : ""
  assertBuyback(mime === input.mime_type && bytes.length <= 3 * 1024 * 1024, "BUYBACK_INVALID_PHOTO")
  const files = container.resolve<IFileModuleService>(Modules.FILE)
  const file = await files.createFiles({ filename: `buyback-${randomUUID()}.${mime === "image/jpeg" ? "jpg" : mime.split("/")[1]}`, mimeType: mime, content: input.content, access: "private" })
  try {
    const photo = await service.createBuybackPhotos({ request_id: request.id, item_id: input.item_id, photo_type: input.photo_type, uploaded_at: new Date(), storage_key: file.id, mime_type: mime })
    return new StepResponse(publicPhoto(photo), { photo_id: photo.id, file_id: file.id })
  } catch (error) { await files.deleteFiles(file.id); throw error }
}, async (data: { photo_id: string; file_id: string } | undefined, { container }) => {
  if (!data) return
  await container.resolve<BuybackModuleService>(BUYBACK_MODULE).deleteBuybackPhotos(data.photo_id)
  await container.resolve<IFileModuleService>(Modules.FILE).deleteFiles(data.file_id)
})
export const uploadBuybackPhotoWorkflow = createWorkflow("upload-buyback-photo", function (input: PhotoInput) {
  acquireLockStep({ key: input.request_id, timeout: 10, ttl: 60 })
  const photo = uploadBuybackPhoto(input)
  releaseLockStep({ key: input.request_id })
  return new WorkflowResponse(photo)
})

type PriceInput = BuybackReferencePrice & { actor_id: string }
const updateBuybackReferencePrice = createStep("update-buyback-reference-price", async (input: PriceInput, { container }) => {
  await adminBuybackAccess(container, input.actor_id)
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: products } = await query.graph({ entity: "product", fields: ["id", "title", "options.title", "options.values.value"], filters: { id: input.product_id } })
  assertBuyback(/^iPhone\b/i.test(products[0]?.title || "") && products[0]?.options?.some((option) => /storage|capacity|memory|容量/i.test(option.title) && option.values?.some((value) => normalizeCapacity(value.value) === normalizeCapacity(input.capacity))), "BUYBACK_INVALID_MODEL")
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const [existing] = await service.listBuybackPrices({ product_id: input.product_id, capacity: normalizeCapacity(input.capacity) })
  const rules = existing?.pricing_rules ? structuredClone(existing.pricing_rules) as PricingRules : null
  if (rules) {
    const delta = input.amount - rules.bases.LIKE_NEW
    for (const visual of Object.keys(rules.bases)) rules.bases[visual] = Math.max(0, Math.round((rules.bases[visual] + delta) * 100) / 100)
    rules.floor = Math.min(rules.floor, rules.bases.MODERATE)
  }
  const data = { amount: input.amount, ...(rules ? { pricing_rules: rules as unknown as Record<string, unknown> } : {}) }
  const price = existing ? await service.updateBuybackPrices({ id: existing.id, ...data }) : await service.createBuybackPrices({ product_id: input.product_id, capacity: normalizeCapacity(input.capacity), ...data })
  return new StepResponse(price, { id: price.id, previous: existing?.amount, pricing_rules: existing?.pricing_rules })
}, async (before: { id: string; previous?: number; pricing_rules?: Record<string, unknown> | null } | undefined, { container }) => {
  if (!before) return
  const service = container.resolve<BuybackModuleService>(BUYBACK_MODULE)
  if (before.previous == null) await service.deleteBuybackPrices(before.id)
  else await service.updateBuybackPrices({ id: before.id, amount: before.previous, pricing_rules: before.pricing_rules })
})
export const setBuybackPriceWorkflow = createWorkflow("set-buyback-price", function (input: PriceInput) {
  const key = "buyback-price-catalog"
  acquireLockStep({ key, timeout: 10, ttl: 60 })
  const result = updateBuybackReferencePrice(input)
  releaseLockStep({ key })
  return new WorkflowResponse(result)
})
