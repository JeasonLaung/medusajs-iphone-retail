// Run against the local storefront/backend with PLAYWRIGHT_MODULE_PATH set.
const { test, before, after } = require("node:test")
const assert = require("node:assert/strict")

const enabled = Boolean(process.env.PLAYWRIGHT_MODULE_PATH)
const baseUrl = process.env.STOREFRONT_TEST_URL || "http://127.0.0.1:8000"
let browser
before(async () => {
  if (enabled) browser = await require(process.env.PLAYWRIGHT_MODULE_PATH).chromium.launch({ headless: true })
})
after(async () => { await browser?.close() })

for (const language of ["en", "zh"]) {
  for (const query of ["", "?color=Silver,Gold", "?page=2", "?q=iPhone+15", "?color=NonexistentColour"]) {
    test(`first HTML and hydration agree: ${language} ${query}`, { skip: !enabled }, async () => {
      const page = await browser.newPage()
      try {
        const requests = []
        const errors = []
        page.on("request", (request) => {
          if (new URL(request.url()).pathname === "/store/search") requests.push(request.postDataJSON())
        })
        page.on("pageerror", (error) => errors.push(error.message))
        const response = await page.goto(`${baseUrl}/dk-${language}/store${query}`, { waitUntil: "networkidle" })
        assert.equal(response.status(), 200)
        const html = await response.text()
        const htmlCards = (html.match(/data-testid="product-title"/g) || []).length
        assert.equal(await page.getByTestId("product-title").count(), htmlCards)
        if (!query.includes("Nonexistent")) assert(htmlCards > 0)
        assert.deepEqual(requests, [], "hydration must not repeat the server search")
        assert.deepEqual(errors, [])
      } finally { await page.close() }
    })
  }
}

test("products, selected filters and the histogram render without JavaScript", { skip: !enabled }, async () => {
  const context = await browser.newContext({ javaScriptEnabled: false })
  try {
    const page = await context.newPage()
    await page.goto(`${baseUrl}/dk-zh/store?color=Silver,Gold`, { waitUntil: "networkidle" })
    assert(await page.getByTestId("product-title").count() > 0)
    assert.equal(await page.locator('[data-testid="refinement-color_families"][aria-pressed="true"]').count(), 2)
    const count = await page.locator('[data-testid="price-histogram"] span').evaluateAll(
      (bins) => bins.reduce((sum, bin) => sum + Number(bin.dataset.count), 0)
    )
    assert(count > 0)
  } finally { await context.close() }
})

test("card capacities and colours use live stock despite stale search data", { skip: !enabled }, async () => {
  for (const language of ["en", "zh"]) {
    const page = await browser.newPage()
    try {
      await page.goto(`${baseUrl}/dk-${language}/store`, { waitUntil: "networkidle" })
      // Exercise a partial SKU catalogue without changing real product data.
      await page.route("**/store/search", async (route) => {
        const response = await route.fetch()
        const body = await response.json()
        if (route.request().postDataJSON().queries?.[0]?.pagination?.take === 50) {
          const document = body.results[0].hits[0]?.document
          assert(document, "the fixture needs one matching product")
          Object.assign(document, {
            model_storage_options: ["64 GB", "128 GB", "256 GB"],
            sku_storage_options: ["64 GB", "128 GB"],
            stocked_storage_options: ["64 GB", "128 GB", "256 GB"],
            color_options: ["Black", "White", "Pink"],
            color_hexes: ["#222222", "#f9f8f4", "#ef9cae"],
            sku_color_options: ["Black", "White"],
          })
        }
        await route.fulfill({ response, json: body })
      })
      await page.route("**/store/products?**", async (route) => {
        const response = await route.fetch()
        const body = await response.json()
        for (const product of body.products) {
          product.options = [{ id: "storage", title: "Storage" }, { id: "color", title: "Color" }]
          product.variants = [["64 GB", "Black", 2], ["128 GB", "White", 0], ["256 GB", "Pink", 0]].map(([storage, color, stock], index) => ({
            id: `fixture-${index}`, inventory_quantity: stock,
            options: [{ option_id: "storage", value: storage }, { option_id: "color", value: color }],
          }))
        }
        await route.fulfill({ response, json: body })
      })
      await page.getByTestId("store-search-input").fill("iPhone 11")
      const card = page.getByTestId("product-wrapper").first()
      const missingStorage = card.getByTestId("product-storage").filter({ hasText: "256 GB" })
      await missingStorage.waitFor()
      await page.waitForFunction(() => document.querySelector('[data-testid="product-storage"][aria-disabled="true"]'))
      assert.equal(await missingStorage.getAttribute("aria-disabled"), "true")
      assert.equal(await missingStorage.evaluate((element) => getComputedStyle(element).textDecorationLine), "line-through")
      assert.equal(await card.getByTestId("product-storage").filter({ hasText: "64 GB" }).getAttribute("aria-disabled"), null)
      assert.equal(await card.getByTestId("product-storage").filter({ hasText: "128 GB" }).getAttribute("aria-disabled"), "true")
      const missingColor = card.getByTestId("product-color").filter({ has: page.locator('[aria-hidden="true"]') })
      assert.equal(await missingColor.count(), 2)
      for (const swatch of await missingColor.all()) {
        assert.equal(await swatch.getAttribute("aria-disabled"), "true")
        assert.equal(await swatch.evaluate((element) => getComputedStyle(element).opacity), "0.4")
        assert.match(await swatch.getAttribute("title"), language === "zh" ? /暂无库存/ : /Out of stock/)
      }
      assert.equal(await card.getByTestId("product-color").count(), 3)
    } finally { await page.close() }
  }
})

test("pagination, refinements, history, sorting and search use API updates", { skip: !enabled }, async () => {
  const page = await browser.newPage()
  try {
    const requests = []
    const errors = []
    let pageRequests = 0
    page.on("request", (request) => {
      if (request.resourceType() === "document" || request.headers().rsc) pageRequests++
      if (new URL(request.url()).pathname === "/store/search") requests.push(request.postDataJSON())
    })
    page.on("pageerror", (error) => errors.push(error.message))
    await page.goto(`${baseUrl}/dk-en/store`, { waitUntil: "networkidle" })
    const initialPageRequests = pageRequests
    async function settle(expectedCount) {
      // Include the histogram debounce and delayed URL write in each observation.
      await page.waitForTimeout(800)
      await page.waitForLoadState("networkidle")
      assert.equal(requests.length, expectedCount)
      assert.equal(new Set(requests.map((request) => JSON.stringify(request))).size, requests.length)
      requests.length = 0
    }
    await page.getByTestId("product-pagination").getByRole("button", { name: "2", exact: true }).click()
    await settle(1)
    assert.equal(new URL(page.url()).searchParams.get("page"), "2")
    const colors = page.getByTestId("filter-group-color_families")
    await colors.getByRole("button", { name: "Silver", exact: true }).click()
    await settle(2) // Product results and a separate complete price distribution.
    assert.equal(new URL(page.url()).searchParams.get("color"), "Silver")
    await colors.getByRole("button", { name: "Gold", exact: true }).click()
    await settle(2)
    assert.match(page.url(), /color=(Silver,Gold|Gold,Silver)/)
    await page.goBack()
    await settle(2)
    assert.equal(new URL(page.url()).searchParams.get("color"), "Silver")
    await page.getByTestId("clear-refinements").click()
    await settle(1)
    assert.equal(new URL(page.url()).search, "")
    await page.getByTestId("sort-by-container").selectOption("product/sort/min_price_eur:asc")
    await settle(1)
    await page.getByTestId("store-search-input").fill("iPhone 15")
    await settle(2)
    assert.equal(new URL(page.url()).searchParams.get("q"), "iPhone 15")
    assert.equal(pageRequests, initialPageRequests, "filter actions must not request a new page")
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})
