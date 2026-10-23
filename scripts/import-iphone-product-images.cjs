// Preview: node scripts/import-iphone-product-images.cjs <source-directory>
// Apply:   node scripts/import-iphone-product-images.cjs <source-directory> --apply
const fs = require("node:fs")
const path = require("node:path")
const { createHash } = require("node:crypto")
const { createRequire } = require("node:module")
const root = path.resolve(__dirname, "..")
const backend = path.join(root, "medusa-store/apps/backend")
const backendRequire = createRequire(path.join(backend, "package.json"))
const Medusa = backendRequire("@medusajs/js-sdk").default
const slug = (value) => value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")
const colorSlug = (value) => slug(value) === "product-red" ? "red" : slug(value)
const imageKey = (url) => {
  const match = url.match(/\/iphone-products\/([^/]+)\/([^/]+)\/[^/?]+(?:\?.*)?$/)
  return match ? `${match[1]}/${colorSlug(match[2])}` : null
}
const hash = (file) => createHash("sha256").update(fs.readFileSync(file)).digest("hex")

async function retryConnection(operation) {
  for (let attempt = 0; ; attempt++) {
    try { return await operation() } catch (error) {
      if (attempt >= 6 || error.message !== "fetch failed") throw error
      await new Promise((done) => setTimeout(done, 5000))
    }
  }
}

async function main() {
  const sourceArg = process.argv[2]
  if (!sourceArg || sourceArg.startsWith("--")) throw new Error("Supply the source iphone-products directory")
  const source = fs.realpathSync(sourceArg)
  const apply = process.argv.includes("--apply")
  const baseUrl = process.env.IPHONE_IMAGE_BACKEND_URL || "http://localhost:9001"
  const sdk = new Medusa({ baseUrl, auth: { type: "jwt", jwtTokenStorageMethod: "memory" } })
  const credentials = fs.readFileSync(path.join(root, ".admin-credentials"), "utf8")
  const email = credentials.match(/EMAIL\s*[:=]\s*(.+)/i)?.[1].trim()
  const password = credentials.match(/PASSWORD\s*[:=]\s*(.+)/i)?.[1].trim()
  if (!email || !password) throw new Error("Missing local admin credentials")
  const token = await retryConnection(() => sdk.auth.login("user", "emailpass", { email, password }))
  if (typeof token !== "string") throw new Error("Admin login requires additional authentication")
  const request = (url, method = "GET", body, query) => retryConnection(() => sdk.client.fetch(url, {
    method, ...(body ? { body } : {}), ...(query ? { query } : {}), headers: { Authorization: `Bearer ${token}` },
  }))
  const groups = new Map()
  const copies = []
  for (const model of fs.readdirSync(source, { withFileTypes: true }).filter((entry) => entry.isDirectory())) {
    if (!/^iphone-[a-z0-9-]+$/.test(model.name)) continue
    for (const color of fs.readdirSync(path.join(source, model.name), { withFileTypes: true }).filter((entry) => entry.isDirectory())) {
      if (!/^[a-z0-9-]+$/.test(color.name)) continue
      const files = fs.readdirSync(path.join(source, model.name, color.name)).filter((file) => /^\d+\.(webp|png|jpe?g)$/i.test(file)).sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))
      if (!files.length) continue
      const key = `${model.name}/${colorSlug(color.name)}`
      if (groups.has(key)) throw new Error(`Ambiguous image group: ${key}`)
      groups.set(key, files.map((file) => `${baseUrl}/static/iphone-products/${model.name}/${color.name}/${file}`))
      for (const file of files) {
        const from = path.join(source, model.name, color.name, file)
        const to = path.join(backend, "static/iphone-products", model.name, color.name, file)
        if (fs.existsSync(to) && hash(from) !== hash(to)) throw new Error(`Refusing to overwrite a different existing image: ${to}`)
        copies.push({ from, to })
      }
    }
  }
  const products = []
  for (let offset = 0; ; offset += 100) {
    const result = await request("/admin/products", "GET", null, { limit: 100, offset, fields: "id,handle,title" })
    products.push(...result.products)
    if (offset + result.products.length >= result.count) break
  }
  const plans = []
  const unmatched = []
  for (const item of products) {
    const model = item.handle.replace(/^swappie-/, "")
    if (!model.startsWith("iphone-")) continue
    const { product } = await request(`/admin/products/${item.id}`, "GET", null, { fields: "id,title,handle,thumbnail,*images,*options,*variants,*variants.options,*variants.images" })
    const colors = new Map(product.options.map((option) => [option.id, option.title]))
    const variants = product.variants.map((variant) => {
      const color = variant.options?.find((value) => /^(colou?r|颜色)$/i.test(colors.get(value.option_id) || value.option?.title || ""))?.value
      const key = color ? `${model}/${colorSlug(color)}` : null
      const urls = key ? groups.get(key) : null
      if (!urls) unmatched.push({ model, sku: variant.sku, color: color || null })
      return { ...variant, urls, key }
    })
    const urls = [...new Set(variants.flatMap((variant) => variant.urls || []))]
    if (urls.length) plans.push({ product, variants, urls })
  }
  const summary = { files: copies.length, groups: groups.size, products: plans.length, matchedSkus: plans.reduce((count, plan) => count + plan.variants.filter((variant) => variant.urls).length, 0), unmatchedSkus: unmatched.length, missingColorSkus: unmatched.filter((variant) => !variant.color).length, missingImageModels: [...new Set(unmatched.filter((variant) => variant.color).map((variant) => variant.model))] }
  console.log(JSON.stringify({ mode: apply ? "apply" : "preview", ...summary }, null, 2))
  if (!apply) return
  fs.mkdirSync(path.join(root, "logs"), { recursive: true })
  const backup = path.join(root, "logs", `iphone-image-import-${Date.now()}.json`)
  fs.writeFileSync(backup, JSON.stringify({ summary, unmatched, products: plans.map(({ product }) => product) }, null, 2))
  for (const { from, to } of copies) {
    fs.mkdirSync(path.dirname(to), { recursive: true })
    if (!fs.existsSync(to)) fs.copyFileSync(from, to, fs.constants.COPYFILE_EXCL)
    if (hash(from) !== hash(to)) throw new Error(`Image copy verification failed: ${to}`)
  }
  for (const { product, variants, urls } of plans) {
    const existing = product.images || []
    const newUrls = urls.filter((url) => !existing.some((image) => image.url === url))
    if (newUrls.length || !urls.includes(product.thumbnail)) {
      await request(`/admin/products/${product.id}`, "POST", { images: [...existing.map(({ id, url }) => ({ id, url })), ...newUrls.map((url) => ({ url }))], thumbnail: urls.includes(product.thumbnail) ? product.thumbnail : urls[0] })
    }
    const { product: updated } = await request(`/admin/products/${product.id}`, "GET", null, { fields: "id,*images,*images.variants" })
    for (const image of updated.images) {
      const key = imageKey(image.url)
      // Medusa exposes unassigned images as general images on every SKU.
      // Bind old general images to the legacy SKUs that still use them.
      const assigned = new Set((image.variants || []).map((variant) => variant.id))
      const desired = variants.filter((variant) => key ? variant.key === key && variant.urls?.includes(image.url) : !variant.urls).map((variant) => variant.id)
      const add = desired.filter((id) => !assigned.has(id))
      const remove = variants.filter((variant) => variant.urls && !desired.includes(variant.id) && assigned.has(variant.id)).map((variant) => variant.id)
      if (add.length || remove.length) await request(`/admin/products/${product.id}/images/${image.id}/variants/batch`, "POST", { add, remove })
    }
    const updates = variants.filter((variant) => variant.urls && variant.thumbnail !== variant.urls[0]).map((variant) => ({ id: variant.id, thumbnail: variant.urls[0] }))
    for (let index = 0; index < updates.length; index += 100) await request(`/admin/products/${product.id}/variants/batch`, "POST", { update: updates.slice(index, index + 100) })
    console.log(`${product.handle}: ${variants.filter((variant) => variant.urls).length} SKUs mapped`)
  }
  console.log(`Complete. Original image assignments saved in ${path.relative(root, backup)}`)
}

main().catch((error) => { console.error(error.message); process.exitCode = 1 })
