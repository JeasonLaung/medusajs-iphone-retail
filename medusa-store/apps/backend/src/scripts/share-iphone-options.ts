import type { Context, ExecArgs, IProductModuleService, ProductDTO } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError, Modules } from "@medusajs/framework/utils"
import { createStep, createWorkflow, StepResponse, WorkflowResponse } from "@medusajs/framework/workflows-sdk"
import { createProductOptionsWorkflow, emitEventStep, updateProductOptionsWorkflow } from "@medusajs/medusa/core-flows"
import assert from "node:assert/strict"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { resolve } from "node:path"

// pnpm exec medusa exec ./src/scripts/share-iphone-options.ts [apply]
// A data migration for Medusa 2.21.2, not a second development server.
const titles = ["Storage", "Condition", "Battery", "Color"]
const relations = ["options.values", "variants.options"]
type Link = { product_id: string; product_option_id: string; product_option_value_ids: string[] }
type VariantLink = { id: string; options: { id: string }[] }
// The public variant writer requires complete option tuples. Historical source
// variants have only Storage, so this migration uses the module's relation CRUD
// service inside its own transaction, preserving those intentionally partial rows.
type MigrationService = IProductModuleService & {
  baseRepository_: { transaction<T>(task: (manager: Context["transactionManager"]) => Promise<T>): Promise<T> }
  productVariantService_: { upsertWithReplace(rows: VariantLink[], config: { relations: string[] }, context: Context): Promise<unknown> }
}
type Replacement = { product: ProductDTO; links: Link[]; variants: VariantLink[] }

async function replace(service: MigrationService, input: Replacement, rollbackCheck = false) {
  await service.baseRepository_.transaction(async (transactionManager) => {
    const context: Context = { transactionManager, __type: "MedusaContext" }
    const manager = transactionManager as { flush(): Promise<void>; clear(): void }
    const current = await service.retrieveProduct(input.product.id, { relations }, context)
    const currentIds = new Set(current.options.map((option) => option.id))
    const nextIds = new Set(input.links.map((link) => link.product_option_id))
    const add = input.links.filter((link) => !currentIds.has(link.product_option_id))
    if (add.length) await service.addProductOptionToProduct(add, context)
    await service.productVariantService_.upsertWithReplace(structuredClone(input.variants), { relations: ["options"] }, context)
    // Flush relation replacements before the module's option-removal guard.
    await manager.flush()
    const remove = current.options.filter((option) => !nextIds.has(option.id)).map((option) => ({ product_id: current.id, product_option_id: option.id }))
    if (remove.length) await service.removeProductOptionFromProduct(remove, context)
    await manager.flush()
    manager.clear()
    if (rollbackCheck) throw new MedusaError(MedusaError.Types.INVALID_DATA, "Shared option rollback check")
    const updated = await service.retrieveProduct(current.id, { relations }, context)
    assert.deepEqual(productSignature(updated), productSignature(input.product), `Configuration changed on ${current.handle}`)
  })
}

function oldLinks(product: ProductDTO): Replacement {
  return {
    product,
    links: product.options.map((option) => ({ product_id: product.id, product_option_id: option.id, product_option_value_ids: option.values.map((value) => value.id) })),
    variants: product.variants.map((variant) => ({ id: variant.id, options: variant.options.map((value) => ({ id: value.id })) })),
  }
}

function productSignature(product: ProductDTO) {
  const optionTitles = new Map(product.options.map((option) => [option.id, option.title]))
  return {
    id: product.id,
    options: product.options.map((option) => ({ title: option.title, values: option.values.map((value) => value.value).sort() })).sort((a, b) => a.title.localeCompare(b.title)),
    variants: [...product.variants].sort((a, b) => a.id.localeCompare(b.id)).map((variant) => ({
      id: variant.id, sku: variant.sku, title: variant.title, metadata: variant.metadata,
      manage_inventory: variant.manage_inventory, allow_backorder: variant.allow_backorder,
      options: variant.options.map((value) => ({ title: optionTitles.get(value.option_id ?? ""), value: value.value })).sort((a, b) => String(a.title).localeCompare(String(b.title))),
    })),
  }
}

const shareIphoneOptionLinksStep = createStep("share-iphone-option-links", async (input: Replacement, { container }) => {
  await replace(container.resolve(Modules.PRODUCT) as MigrationService, input)
  return new StepResponse(input.product.id, oldLinks(input.product))
}, async (previous: Replacement | undefined, { container }) => {
  if (!previous) return
  const service = container.resolve(Modules.PRODUCT) as MigrationService
  await service.restoreProductOptions(previous.links.map((link) => link.product_option_id))
  await replace(service, previous)
})

const shareIphoneProductOptionsWorkflow = createWorkflow("share-iphone-product-options", function (input: Replacement) {
  const id = shareIphoneOptionLinksStep(input)
  emitEventStep({ eventName: "product.updated", data: { id } })
  return new WorkflowResponse(id)
})

export default async function shareIphoneOptions({ container, args }: ExecArgs) {
  const service = container.resolve(Modules.PRODUCT) as MigrationService
  assert.equal(typeof service.baseRepository_?.transaction, "function", "Unsupported Medusa product module")
  assert.equal(typeof service.productVariantService_?.upsertWithReplace, "function", "Unsupported Medusa variant service")
  if (args[0] === "restore") {
    const backup = JSON.parse(readFileSync(resolve(args[1]), "utf8")) as { products: ProductDTO[] }
    for (const product of backup.products) {
      await service.restoreProductOptions(product.options.map((option) => option.id))
      await replace(service, oldLinks(product))
    }
    console.log("Restored original option associations from backup")
    return
  }
  const catalogue = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as { products: { slug: string }[] }
  // Accept both catalogue handles and the old import prefix during URL migration.
  const handles = catalogue.products.flatMap((product) => [product.slug, `swappie-${product.slug}`])
  const products = await service.listProducts({ handle: handles }, { relations, take: handles.length })
  assert.equal(products.length, catalogue.products.length, "Missing or duplicate catalogue models")
  let rollbackVerified = false
  for (const product of products) {
    assert.deepEqual(product.options.map((option) => option.title).sort(), [...titles].sort())
    for (const variant of product.variants) {
      assert(variant.options.length > 0, `Unconfigured variant ${variant.id}`)
      for (const value of variant.options) assert(product.options.some((option) => option.id === value.option_id))
    }
  }
  const privateCount = products.reduce((count, product) => count + product.options.filter((option) => option.is_exclusive).length, 0)
  console.log(`${products.length} models; ${privateCount} private option associations; ${products.reduce((count, product) => count + product.variants.length, 0)} variants`)
  if (!args.includes("apply")) return
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const protectedData = async () => {
    const { data } = await query.graph({
      entity: "product_variant", filters: { product_id: products.map((product) => product.id) },
      fields: ["id", "sku", "prices.*", "inventory_items.*", "inventory_items.inventory.location_levels.*"],
      pagination: { take: 10000 },
    })
    assert(data.length < 10000, "Variant pagination required")
    // Relation row order is not meaningful.
    const stable = (value: unknown): unknown => Array.isArray(value)
      ? value.map(stable).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))
      : value && typeof value === "object" ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, entry]) => [key, stable(entry)])) : value
    return stable(data)
  }
  const before = await protectedData()
  const backup = resolve(process.cwd(), `../../../backups/shared-iphone-options-${Date.now()}.json`)
  mkdirSync(resolve(backup, ".."), { recursive: true })
  writeFileSync(backup, JSON.stringify({ products, protected: before }, null, 2))
  console.log(`Backup saved: ${backup}`)
  const shared = new Map<string, ProductDTO["options"][number]>()
  for (const title of titles) {
    const matches = await service.listProductOptions({ title, is_exclusive: false }, { relations: ["values"] })
    assert(matches.length <= 1, `Ambiguous shared ${title}`)
    const values = [...new Set([...(matches[0]?.values ?? []).map((value) => value.value), ...products.flatMap((product) => product.options.filter((option) => option.title === title).flatMap((option) => option.values.map((value) => value.value)))])]
    let option = matches[0]
    if (!option) {
      const { result } = await createProductOptionsWorkflow(container).run({ input: { product_options: [{ title, values, is_exclusive: false }] } })
      option = result[0]
    } else if (values.length !== option.values.length) {
      await updateProductOptionsWorkflow(container).run({ input: { selector: { id: option.id }, update: { values } } })
      option = await service.retrieveProductOption(option.id, { relations: ["values"] })
    }
    shared.set(title, option)
  }
  for (const product of products) {
    const links = product.options.map((option) => {
      const target = shared.get(option.title)!
      const allowed = new Set(option.values.map((value) => value.value))
      return { product_id: product.id, product_option_id: target.id, product_option_value_ids: target.values.filter((value) => allowed.has(value.value)).map((value) => value.id) }
    })
    if (product.options.some((option) => !links.some((link) => link.product_option_id === option.id))) {
      const variants = product.variants.map((variant) => ({ id: variant.id, options: variant.options.map((value) => {
        const title = product.options.find((option) => option.id === value.option_id)!.title
        const target = shared.get(title)!.values.find((entry) => entry.value === value.value)
        assert(target, `Missing shared value ${title}/${value.value}`)
        return { id: target.id }
      }) }))
      const input = { product, links, variants }
      if (!rollbackVerified) {
        await assert.rejects(replace(service, input, true), /Shared option rollback check/)
        const afterRollback = await service.retrieveProduct(product.id, { relations })
        assert.deepEqual(productSignature(afterRollback), productSignature(product))
        assert.deepEqual(oldLinks(afterRollback).links.sort((a, b) => a.product_option_id.localeCompare(b.product_option_id)), oldLinks(product).links.sort((a, b) => a.product_option_id.localeCompare(b.product_option_id)))
        rollbackVerified = true
        console.log("Verified transaction rollback after relation writes")
      }
      await shareIphoneProductOptionsWorkflow(container).run({ input })
    }
    const updated = await service.retrieveProduct(product.id, { relations })
    assert.deepEqual(productSignature(updated), productSignature(product))
    assert(updated.options.every((option) => !option.is_exclusive && option.id === shared.get(option.title)!.id))
    console.log(`Verified ${product.handle}: ${updated.variants.length} original and complete variants preserved`)
  }
  assert.deepEqual(await protectedData(), before, "Prices or inventory changed during migration")
  console.log("VERIFIED: 4 shared options; per-model subsets, all SKUs, prices, stock and reservations preserved")
}
