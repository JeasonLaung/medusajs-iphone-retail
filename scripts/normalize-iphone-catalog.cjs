// Preview: node scripts/normalize-iphone-catalog.cjs
// Apply through the running backend's native workflows: add --apply.
const fs = require("node:fs")
const path = require("node:path")
const assert = require("node:assert/strict")
const { createRequire } = require("node:module")
const root = path.resolve(__dirname, "..")
const backendRequire = createRequire(path.join(root, "medusa-store/apps/backend/package.json"))
const Medusa = backendRequire("@medusajs/js-sdk").default
const imageUrl = (url) => url?.replace(/\/swappie\/(?:swappie-)?/g, "/product-images/")
const fields = "id,title,handle,description,thumbnail,metadata,*images,*variants,*variants.options,*variants.prices,*variants.images,*variants.inventory_items"
const signature = (product) => JSON.stringify(product.variants.map((v) => ({
  id: v.id, sku: v.sku, metadata: v.metadata, options: v.options,
  prices: v.prices, inventory: v.inventory_items, images: v.images?.map((i) => i.id).sort(),
  manage_inventory: v.manage_inventory, allow_backorder: v.allow_backorder,
})).sort((a, b) => a.id.localeCompare(b.id)))

async function main() {
  const credentials = fs.readFileSync(path.join(root, ".admin-credentials"), "utf8")
  const credential = (key) => credentials.match(new RegExp(`^${key}\\s*[:=]\\s*(.*)$`, "im"))?.[1]?.trim().replace(/^['"]|['"]$/g, "")
  const sdk = new Medusa({ baseUrl: "http://127.0.0.1:9001", auth: { type: "jwt" } })
  const token = await sdk.auth.login("user", "emailpass", { email: credential("email"), password: credential("password") })
  assert.equal(typeof token, "string", "Admin login failed")
  const headers = { authorization: `Bearer ${token}` }
  const products = []
  for (let offset = 0; ; offset += 100) {
    const result = await sdk.admin.product.list({ limit: 100, offset, fields }, headers)
    products.push(...result.products)
    if (offset + result.products.length >= result.count) break
  }
  const plans = products.filter((p) => /^swappie-iphone-/.test(p.handle)).map((product) => {
    const handle = product.handle.replace(/^swappie-/, "")
    assert(!products.some((p) => p.id !== product.id && p.handle === handle), `Handle conflict: ${handle}`)
    for (const image of product.images) {
      if (!image.url.includes("/swappie/")) continue
      const file = path.basename(new URL(imageUrl(image.url), "http://localhost").pathname)
      assert(fs.existsSync(path.join(root, "medusa-store/apps/storefront/public/product-images", file)), `Missing image: ${file}`)
    }
    const metadata = { ...product.metadata, source: "iphone-catalog-snapshot" }
    for (const [key, value] of Object.entries(metadata)) {
      if (/swappie/i.test(JSON.stringify(value))) metadata[key] = null
    }
    return { product, update: {
      handle, metadata, thumbnail: imageUrl(product.thumbnail),
      images: product.images.map((image) => ({ id: image.id, url: imageUrl(image.url) })),
      description: product.description?.replace(/Swappie category snapshot/gi, "iPhone catalog snapshot"),
    } }
  })
  console.log(`${plans.length} products to normalize`)
  if (!process.argv.includes("--apply") || !plans.length) return
  const backup = path.join(root, "logs", `iphone-catalog-before-normalize-${Date.now()}.json`)
  fs.writeFileSync(backup, JSON.stringify(plans.map(({ product }) => product), null, 2))
  console.log(`Backup saved: ${backup}`)
  for (const { product, update } of plans) {
    await sdk.admin.product.update(product.id, update, {}, headers)
    const { product: after } = await sdk.admin.product.retrieve(product.id, { fields }, headers)
    assert.equal(after.handle, update.handle)
    assert.equal(after.title, product.title)
    assert.equal(signature(after), signature(product), `Variant data changed: ${after.handle}`)
    assert(!/swappie/i.test(JSON.stringify({ handle: after.handle, thumbnail: after.thumbnail, images: after.images, metadata: after.metadata })), `Unclean product: ${after.handle}`)
    console.log(`Normalized ${after.handle}; variant IDs, prices, inventory links and image associations unchanged`)
  }
}
main().catch((error) => { console.error(error.message); process.exitCode = 1 })
