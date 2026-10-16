import type { SearchTypes } from "@medusajs/framework/types";
import {
  defineSearchIndex,
  graphConsume,
  graphSeed,
  search,
} from "@medusajs/framework/utils";

import { ProductOptionRow, toOptionValues } from "./helpers/option-values";
import { loadPricing, priceFields, toProductPricing } from "./helpers/pricing";
import { resolveProductIds } from "./helpers/resolve-product-ids";
import { toColorFamilies } from "./helpers/color-families";
import { cardSkuOptions } from "./helpers/card-sku-options"
import { loadStockedVariantIds, stockedStorageOptions } from "./helpers/stocked-options"

const PRODUCT_GRAPH_FIELDS = [
  "id",
  "title",
  "description",
  "handle",
  "thumbnail",
  "status",
  "created_at",
  "metadata",
  "sales_channels.id",
  "categories.name",
  "tags.value",
  "options.title",
  "options.values.value",
  "variants.options.value",
  "variants.sku",
  "variants.id",
  "variants.options.option.title",
];

type ProductRow = {
  id: string;
  title?: string | null;
  description?: string | null;
  handle?: string | null;
  thumbnail?: string | null;
  status?: string | null;
  created_at?: string | Date | null;
  deleted_at?: string | Date | null;
  metadata?: Record<string, unknown> | null;
  sales_channels?: ({ id?: string | null } | null)[] | null;
  categories?: ({ name?: string | null } | null)[] | null;
  tags?: ({ value?: string | null } | null)[] | null;
  options?: ProductOptionRow[] | null;
  variants?: ({ id?: string; sku?: string | null; options?: ({ value?: string | null; option?: { title?: string | null } | null } | null)[] | null } | null)[] | null;
};

const productFields = search.define({
  id: search.keyword().filterable().retrievable(),
  // Read by `/store/search` to scope the index; never returned to hits.
  status: search.keyword().filterable().retrievable(false),
  sales_channel_ids: search.keyword().array().filterable().retrievable(false),
  title: search.text().searchable({ weight: 3 }).sortable().retrievable(),
  description: search.text().searchable({ weight: 1 }),
  handle: search.keyword().retrievable(),
  // Returned on hits only: never filtered, sorted or faceted on.
  thumbnail: search.keyword().retrievable(),
  created_at: search.date().sortable().retrievable(),
  category: search.keyword().array().filterable().facetable().retrievable(),
  labels: search.keyword().array().filterable().facetable().retrievable(),
  option_values: search
    .keyword()
    .array()
    .searchable({ weight: 2 })
    .filterable()
    .facetable()
    .retrievable(),
  model: search.keyword().filterable().facetable().retrievable(),
  storage_options: search
    .keyword()
    .array()
    .filterable()
    .facetable()
    .retrievable(),
  color_options: search
    .keyword()
    .array()
    .filterable()
    .facetable()
    .retrievable(),
  color_hexes: search.keyword().array().retrievable(),
  model_storage_options: search.keyword().array().retrievable(),
  sku_storage_options: search.keyword().array().retrievable(),
  stocked_storage_options: search.keyword().array().retrievable(),
  sku_color_options: search.keyword().array().retrievable(),
  color_families: search
    .keyword()
    .array()
    .filterable()
    .facetable()
    .retrievable(),
  battery_options: search
    .keyword()
    .array()
    .filterable()
    .facetable()
    .retrievable(),
  condition_options: search
    .keyword()
    .array()
    .filterable()
    .facetable()
    .retrievable(),
  release_year: search.keyword().filterable().facetable().retrievable(),
  display_size: search.keyword().filterable().facetable().retrievable(),
  rear_cameras: search.keyword().filterable().facetable().retrievable(),
  ...priceFields,
});

type ProductDocument = SearchTypes.InferSearchDocumentType<
  typeof productFields
>;

function toDocument(
  product: ProductRow,
  pricing: Parameters<typeof toProductPricing>[0],
  stockedIds: Set<string>,
): ProductDocument {
  const category = (product.categories ?? [])
    .map((productCategory) => productCategory?.name?.trim())
    .filter((name): name is string => Boolean(name));
  const labels = (product.tags ?? [])
    .map((tag) => tag?.value?.trim())
    .filter((value): value is string => Boolean(value));
  const salesChannelIds = (product.sales_channels ?? [])
    .map((salesChannel) => salesChannel?.id?.trim())
    .filter((id): id is string => Boolean(id));
  const cardColors = Array.isArray(product.metadata?.card_colors)
    ? product.metadata.card_colors.filter(
        (color): color is { name: string; hex: string } =>
          !!color &&
          typeof color === "object" &&
          typeof color.name === "string" &&
          typeof color.hex === "string",
      )
    : [];

  return {
    id: product.id,
    status: product.status ?? null,
    sales_channel_ids: salesChannelIds,
    title: product.title ?? null,
    description: product.description ?? null,
    handle: product.handle ?? null,
    thumbnail: product.thumbnail ?? null,
    created_at: product.created_at ?? null,
    category,
    labels,
    option_values: toOptionValues(product.options),
    model:
      product.metadata?.source === "swappie-html-snapshot"
        ? (product.title ?? null)
        : null,
    // Only capacities attached to real variants belong in the shopping filter.
    // Model-level available_storages remains reference data, not a sale offer.
    storage_options: Array.from(new Set((product.variants ?? []).flatMap((variant) =>
      (variant?.options ?? []).flatMap((option) => {
        const value = option?.value?.trim();
        return value && /^(storage|capacity|容量)$/i.test(option?.option?.title?.trim() ?? "")
          ? [value]
          : [];
      }),
    ))),
    color_options: cardColors.map((color) => color.name),
    color_families: toColorFamilies(cardColors),
    color_hexes: cardColors.map((color) => color.hex),
    ...cardSkuOptions(product.variants, product.metadata?.available_storages),
    stocked_storage_options: stockedStorageOptions(product, stockedIds),
    battery_options: Array.isArray(product.metadata?.available_batteries)
      ? product.metadata.available_batteries.filter(
          (value): value is string => typeof value === "string",
        )
      : [],
    condition_options: Array.isArray(product.metadata?.available_conditions)
      ? product.metadata.available_conditions.filter(
          (value): value is string => typeof value === "string",
        )
      : [],
    release_year:
      typeof product.metadata?.release_year === "number"
        ? String(product.metadata.release_year)
        : null,
    display_size:
      typeof product.metadata?.display_size_inches === "number"
        ? `${product.metadata.display_size_inches} inches`
        : null,
    rear_cameras:
      typeof product.metadata?.rear_camera_count === "number"
        ? `${product.metadata.rear_camera_count} rear camera${product.metadata.rear_camera_count === 1 ? "" : "s"}`
        : null,
    ...toProductPricing(pricing),
  };
}

/**
 * A row that returns no document leaves
 * the index: the helpers turn it into a delete on `consume` and on the
 * catch-up pass.
 */
const source = {
  fields: PRODUCT_GRAPH_FIELDS,
  transform: async (
    rows: ProductRow[],
    context: SearchTypes.SearchIngestionContext,
  ) => {
    const [pricing, stockedIds] = await Promise.all([
      loadPricing(rows.map((row) => row.id), context),
      loadStockedVariantIds(rows, context),
    ]);

    return rows.map((row) => toDocument(row, pricing.get(row.id), stockedIds));
  },
};

/**
 * Everything a document is built from, so a change to any of it re-indexes the
 * products it touches. Only `product.deleted` removes documents: a deleted
 * variant, option, tag or category means the product has to be re-read.
 *
 * TODO: price lists emit no events (create, update, delete, batch prices).
 * TODO: price lists crossing their start or end date change prices silently.
 * TODO: linkProductsToSalesChannelWorkflow emits nothing.
 * TODO: batchLinkProductsToCategoryWorkflow emits nothing.
 * TODO: upsertVariantPricesWorkflow emits nothing when called directly.
 */
const PRODUCT_EVENTS = [
  "product.created",
  "product.updated",
  "product.deleted",
  "product-variant.created",
  "product-variant.updated",
  "product-variant.deleted",
  "product-option.created",
  "product-option.updated",
  "product-option.deleted",
  "product-option-value.updated",
  "product-option-value.deleted",
  "product-tag.updated",
  "product-tag.deleted",
  "product-category.updated",
  "product-category.deleted",
  "sales-channel.deleted",
  "inventory-level.created",
  "inventory-level.updated",
  "inventory-level.deleted",
  "reservation-item.created",
  "reservation-item.updated",
  "reservation-item.deleted",
];

export default defineSearchIndex({
  name: "product",
  entity: "product",
  primary_key: "id",
  fields: productFields,
  settings: {
    typo_tolerance: { enabled: true },
  },
  events: PRODUCT_EVENTS,
  consume: graphConsume<typeof productFields, ProductRow>({
    ...source,
    resolve_ids: resolveProductIds,
    is_delete: (event) => event.name === "product.deleted",
  }),
  seed: graphSeed<typeof productFields, ProductRow>(source),
});
