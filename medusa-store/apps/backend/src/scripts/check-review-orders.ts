import assert from "node:assert/strict"
import type { ExecArgs } from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  generateJwtToken,
  Modules,
  PaymentCollectionStatus,
} from "@medusajs/framework/utils"
import Medusa from "@medusajs/js-sdk"
import { PRODUCT_REVIEW_MODULE } from "../modules/product-review"
import ProductReviewModuleService from "../modules/product-review/service"
import { moderateProductReviewWorkflow } from "../workflows/product-reviews"

// Isolated fixtures only; existing products are read, never changed.
export default async function checkReviewOrders({ container }: ExecArgs) {
  const customers = container.resolve(Modules.CUSTOMER)
  const orders = container.resolve(Modules.ORDER)
  const payments = container.resolve(Modules.PAYMENT)
  const links = container.resolve(ContainerRegistrationKeys.LINK)
  const reviews: ProductReviewModuleService = container.resolve(
    PRODUCT_REVIEW_MODULE
  )
  const [key] = await container
    .resolve(Modules.API_KEY)
    .listApiKeys({ type: "publishable" })
  const config = container.resolve(ContainerRegistrationKeys.CONFIG_MODULE)
  const sdk = new Medusa({
    baseUrl: "http://127.0.0.1:9001",
    publishableKey: key.token,
  })
  const productQuery = {
    limit: 2,
    fields: "id,title,handle",
  }
  let productPage:
    | Awaited<ReturnType<typeof sdk.store.product.list>>
    | undefined
  for (let attempt = 0; attempt < 60; attempt++) {
    try {
      productPage = await sdk.store.product.list(productQuery)
      break
    } catch (error) {
      if (attempt === 59) throw error
    }
    await new Promise((resolve) => setTimeout(resolve, 1000))
  }
  const products = productPage!.products
  assert(products.length === 2, "Two published products are required")
  const customerIds: string[] = []
  const orderIds: string[] = []
  const collectionIds: string[] = []
  const orderLinks: Record<string, Record<string, string>>[] = []
  let browser: any
  const token = (id: string) =>
    generateJwtToken(
      {
        actor_id: id,
        actor_type: "customer",
        auth_identity_id: "review_order_qa",
        app_metadata: { customer_id: id },
      },
      { secret: config.projectConfig.http.jwtSecret, expiresIn: "10m" }
    )
  const expectStatus = async (request: Promise<unknown>, status: number) => {
    await assert.rejects(
      request,
      (error: { status?: number }) => error.status === status
    )
  }
  try {
    const owner = await customers.createCustomers({
      email: `review-owner-${Date.now()}@example.invalid`,
      first_name: "Review QA",
    })
    customerIds.push(owner.id)
    const other = await customers.createCustomers({
      email: `review-other-${Date.now()}@example.invalid`,
      first_name: "Open QA",
    })
    customerIds.push(other.id)
    const ownerHeaders = { authorization: `Bearer ${token(owner.id)}` }
    const otherHeaders = { authorization: `Bearer ${token(other.id)}` }
    async function makeOrder(
      productId: string,
      paid: boolean,
      canceled = false
    ) {
      const order = await orders.createOrders({
        customer_id: owner.id,
        email: owner.email,
        currency_code: "eur",
        status: canceled ? "canceled" : "pending",
        items: [
          {
            product_id: productId,
            product_title: "Review QA product",
            title: "Review QA product",
            quantity: 1,
            unit_price: 100,
          },
        ],
      })
      orderIds.push(order.id)
      if (paid) {
        const collection = await payments.createPaymentCollections({
          currency_code: "eur",
          amount: 100,
        })
        collectionIds.push(collection.id)
        await payments.updatePaymentCollections(collection.id, {
          status: PaymentCollectionStatus.COMPLETED,
        })
        const link = {
          [Modules.ORDER]: { order_id: order.id },
          [Modules.PAYMENT]: { payment_collection_id: collection.id },
        }
        await links.create(link)
        orderLinks.push(link)
      }
      return order
    }
    const paid = await makeOrder(products[0].id, true)
    const unpaid = await makeOrder(products[0].id, false)
    const canceled = await makeOrder(products[0].id, true, true)
    const wrongProduct = await makeOrder(products[1].id, true)
    const body = {
      product_id: products[0].id,
      rating: 5,
      title: "Review QA",
      content: "An isolated review order integration check.",
    }
    const post = (input: object, headers = ownerHeaders) =>
      sdk.client.fetch("/store/product-reviews", {
        method: "POST",
        body: input,
        headers,
      })
    await expectStatus(
      sdk.client.fetch(`/store/orders/${paid.id}/product-reviews`),
      401
    )
    await expectStatus(
      sdk.client.fetch(`/store/orders/${paid.id}/product-reviews`, {
        headers: otherHeaders,
      }),
      404
    )
    await expectStatus(post({ ...body, order_id: paid.id }, otherHeaders), 404)
    await expectStatus(post({ ...body, order_id: unpaid.id }), 400)
    await expectStatus(post({ ...body, order_id: canceled.id }), 400)
    await expectStatus(post({ ...body, order_id: wrongProduct.id }), 400)
    const eligible = await sdk.client.fetch<{
      orders: { id: string; eligible: boolean }[]
    }>("/store/product-reviews/purchases", {
      query: { product_id: products[0].id },
      headers: ownerHeaders,
    })
    assert.deepEqual(
      eligible.orders
        .filter((order) => order.eligible)
        .map((order) => order.id),
      [paid.id]
    )
    const otherOrders = await sdk.client.fetch<{ orders: unknown[] }>(
      "/store/product-reviews/purchases",
      { query: { product_id: products[0].id }, headers: otherHeaders }
    )
    assert.equal(otherOrders.orders.length, 0)
    console.log(
      "PASS order ownership, paid status, cancellation, product matching and private order list"
    )
    await expectStatus(
      post({ ...body, verified_purchase: true }, otherHeaders),
      400
    )
    await post(body, otherHeaders)
    const [openReview] = await reviews.listProductReviews({
      customer_id: other.id,
    })
    assert.equal(openReview.verified_purchase, false)
    assert.equal(openReview.order_id, null)
    console.log(
      "PASS open reviews remain available and verified purchase cannot be forged"
    )

    if (process.env.PLAYWRIGHT_MODULE_PATH) {
      const { chromium } = require(process.env.PLAYWRIGHT_MODULE_PATH)
      browser = await chromium.launch({ channel: "chrome", headless: true })
      const context = await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
      await context.addCookies([
        {
          name: "_medusa_jwt",
          value: token(owner.id),
          domain: "127.0.0.1",
          path: "/",
          httpOnly: true,
          sameSite: "Lax",
        },
        {
          name: "_medusa_cache_id",
          value: owner.id,
          domain: "127.0.0.1",
          path: "/",
        },
      ])
      const page = await context.newPage()
      const errors: string[] = []
      page.on("pageerror", (error: Error) => errors.push(error.message))
      await page.goto(
        `http://127.0.0.1:8000/dk-zh/products/${products[0].handle}#product-reviews`,
        { waitUntil: "networkidle", timeout: 90000 }
      )
      await page
        .getByTestId("product-reviews")
        .getByRole("button", { name: "写评价", exact: true })
        .click()
      await page.getByTestId("review-form").locator("select:enabled").waitFor()
      assert.equal(
        await page.getByTestId("review-form").locator("option").count(),
        2
      )
      console.log("PASS Chinese product page optional order selector")
      await page.goto(
        `http://127.0.0.1:8000/dk-en/account/orders/details/${paid.id}#order-reviews`,
        { waitUntil: "networkidle" }
      )
      await page
        .getByTestId("order-reviews")
        .getByRole("button", { name: "Write a review", exact: true })
        .click()
      const form = page.getByTestId("review-form")
      await form.locator('input[name="title"]').fill(body.title)
      await form.locator('textarea[name="content"]').fill(body.content)
      await form
        .getByRole("button", { name: "Submit review", exact: true })
        .click()
      await page
        .getByTestId("order-reviews")
        .getByText("Submitted — awaiting moderation", { exact: true })
        .waitFor()
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(
        `http://127.0.0.1:8000/dk-zh/account/orders/details/${paid.id}#order-reviews`,
        { waitUntil: "networkidle" }
      )
      await page
        .getByTestId("order-reviews")
        .getByText("已提交，等待审核", { exact: true })
        .waitFor()
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth
        )
      )
      await page.goto(
        `http://127.0.0.1:8000/dk-zh/account/orders/details/${unpaid.id}#order-reviews`,
        { waitUntil: "networkidle" }
      )
      await page
        .getByTestId("order-reviews")
        .getByText(/订单完成付款后即可评价/)
        .waitFor()
      assert.deepEqual(errors, [])
      console.log(
        "PASS English order submission, Chinese persisted status, mobile layout and unpaid notice"
      )
    } else {
      await post({ ...body, order_id: paid.id })
    }
    const [verified] = await reviews.listProductReviews({
      customer_id: owner.id,
    })
    assert.equal(verified.order_id, paid.id)
    assert.equal(verified.verified_purchase, true)
    assert.equal(verified.status, "pending")
    await expectStatus(post({ ...body, order_id: paid.id }), 409)
    let publicPage = await sdk.client.fetch<{
      reviews: { id: string; verified_purchase: boolean }[]
    }>("/store/product-reviews", {
      query: { product_id: products[0].id, limit: 20 },
    })
    assert.ok(!publicPage.reviews.some((review) => review.id === verified.id))
    await moderateProductReviewWorkflow(container).run({
      input: { id: verified.id, status: "approved" },
    })
    publicPage = await sdk.client.fetch("/store/product-reviews", {
      query: { product_id: products[0].id, limit: 20 },
    })
    const published = publicPage.reviews.find(
      (review) => review.id === verified.id
    )!
    assert.equal(published.verified_purchase, true)
    assert.ok(!("order_id" in published) && !("customer_id" in published))
    if (browser) {
      const page = await browser.newPage({
        viewport: { width: 390, height: 844 },
      })
      await page.goto(
        `http://127.0.0.1:8000/dk-zh/products/${products[0].handle}#product-reviews`,
        { waitUntil: "networkidle" }
      )
      await page
        .getByTestId("product-reviews")
        .getByText("已验证购买", { exact: true })
        .first()
        .waitFor()
      console.log("PASS public verified purchase badge")
    }
    console.log(
      "PASS persisted association, duplicate protection, moderation and public privacy"
    )
  } finally {
    if (browser) await browser.close()
    if (customerIds.length) {
      const fixtures = await reviews.listProductReviews({
        customer_id: customerIds,
      })
      if (fixtures.length)
        await reviews.deleteProductReviews(fixtures.map((review) => review.id))
    }
    for (const link of orderLinks) await links.dismiss(link)
    if (orderIds.length) await orders.deleteOrders(orderIds)
    if (collectionIds.length)
      await payments.deletePaymentCollections(collectionIds)
    if (customerIds.length) await customers.deleteCustomers(customerIds)
    console.log("Review QA fixtures removed")
  }
}
