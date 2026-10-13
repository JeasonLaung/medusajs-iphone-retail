import type { ExecArgs } from "@medusajs/framework/types";
import {
  ContainerRegistrationKeys,
  MedusaError,
  Modules,
  ProductStatus,
} from "@medusajs/framework/utils";
import {
  createProductCategoriesWorkflow,
  createProductsWorkflow,
} from "@medusajs/medusa/core-flows";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

type SourceProduct = {
  model: string;
  slug: string;
  sku: string;
  selected_storage_gb: number;
  available_storages_gb: number[];
  available_colors: string[];
  card_colors: { name: string; hex: string }[];
  price_eur: number;
  source_url: string;
  local_image_url: string;
  source_release_year_code: number;
  release_year: number;
};

type Manifest = {
  source_url: string;
  extracted_at: string;
  source_file_sha256: string;
  products: SourceProduct[];
};

type FilterSupplements = {
  battery_options: string[];
  condition_options: string[];
};

type ModelSpecification = {
  model: string;
  release_year: number;
  apple_capacities: string;
  apple_colors: string[];
  apple_model_numbers: string[];
  display_size_inches: number;
  rear_camera_count: number;
  connector: string | null;
  biometric: string | null;
  display_name: string | null;
  display_resolution_px: string | null;
  pixel_density_ppi: number | null;
  chip: string | null;
  rear_camera_system: string | null;
  front_camera: string | null;
  width_mm: number | null;
  height_mm: number | null;
  depth_mm: number | null;
  weight_g: number | null;
  display_source_url: string;
  camera_source_url: string;
  tech_specs_url: string;
};

type ModelSpecifications = {
  products: ModelSpecification[];
};

export default async function importSwappie({ container }: ExecArgs) {
  const manifestPath = resolve(
    process.cwd(),
    "../../../imports/swappie-iphone-catalog.json",
  );
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as Manifest;
  const supplementsPath = resolve(
    process.cwd(),
    "../../../imports/swappie-filter-supplements.json",
  );
  const supplements = JSON.parse(
    readFileSync(supplementsPath, "utf8"),
  ) as FilterSupplements;
  const specificationsPath = resolve(
    process.cwd(),
    "../../../imports/iphone-model-specs.json",
  );
  const specifications = JSON.parse(
    readFileSync(specificationsPath, "utf8"),
  ) as ModelSpecifications;
  const specsByModel = new Map(
    specifications.products.map((specification) => [
      specification.model,
      specification,
    ]),
  );
  const query = container.resolve(ContainerRegistrationKeys.QUERY);
  const productService = container.resolve(Modules.PRODUCT);

  const { data: stores } = await query.graph({
    entity: "store",
    fields: ["id", "default_sales_channel_id"],
  });
  const salesChannelId = stores[0]?.default_sales_channel_id;
  const { data: shippingProfiles } = await query.graph({
    entity: "shipping_profile",
    fields: ["id"],
  });
  const shippingProfileId = shippingProfiles[0]?.id;
  if (!salesChannelId || !shippingProfileId) {
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      "A store sales channel and shipping profile are required",
    );
  }

  const { data: categories } = await query.graph({
    entity: "product_category",
    fields: ["id", "name"],
  });
  let categoryId = categories.find(
    (category) => category.name === "Swappie iPhones",
  )?.id;
  if (!categoryId) {
    const { result } = await createProductCategoriesWorkflow(container).run({
      input: {
        product_categories: [{ name: "Swappie iPhones", is_active: true }],
      },
    });
    categoryId = result[0].id;
  }

  let created = 0;
  let updated = 0;
  let skipped = 0;
  for (const item of manifest.products) {
    const specification = specsByModel.get(item.model);
    if (!specification) {
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        `Missing Apple specification for ${item.model}`,
      );
    }
    if (specification.release_year !== item.release_year) {
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        `Release year mismatch for ${item.model}`,
      );
    }
    const handle = `swappie-${item.slug}`;
    const existing = await productService.listProducts({ handle });
    if (existing.length) {
      const product = existing[0];
      const currentOptions = product.metadata?.available_batteries;
      if (
        JSON.stringify(currentOptions) !==
          JSON.stringify(supplements.battery_options) ||
        JSON.stringify(product.metadata?.available_conditions) !==
          JSON.stringify(supplements.condition_options) ||
        product.metadata?.display_size_inches !==
          specification.display_size_inches ||
        product.metadata?.rear_camera_count !==
          specification.rear_camera_count ||
        JSON.stringify(product.metadata?.apple_model_specs) !==
          JSON.stringify(specification) ||
        JSON.stringify(product.metadata?.card_colors) !==
          JSON.stringify(item.card_colors) ||
        product.metadata?.release_year !== item.release_year ||
        product.metadata?.source_release_year_code !==
          item.source_release_year_code
      ) {
        await productService.updateProducts(product.id, {
          metadata: {
            ...product.metadata,
            available_batteries: supplements.battery_options,
            available_conditions: supplements.condition_options,
            display_size_inches: specification.display_size_inches,
            rear_camera_count: specification.rear_camera_count,
            apple_model_specs: specification,
            card_colors: item.card_colors,
            display_spec_source_url: specification.display_source_url,
            camera_spec_source_url: specification.camera_source_url,
            release_year: item.release_year,
            source_release_year_code: item.source_release_year_code,
          },
        });
        updated += 1;
      } else {
        skipped += 1;
      }
      continue;
    }

    const storage = `${item.selected_storage_gb} GB`;
    await createProductsWorkflow(container).run({
      input: {
        products: [
          {
            title: item.model,
            handle,
            description:
              "Reference catalog entry from the supplied Swappie category snapshot. " +
              "Only the displayed SKU and starting price were provided. " +
              "Stock quantity has not been verified.",
            status: ProductStatus.PUBLISHED,
            category_ids: [categoryId],
            shipping_profile_id: shippingProfileId,
            sales_channels: [{ id: salesChannelId }],
            thumbnail: item.local_image_url,
            images: [{ url: item.local_image_url }],
            metadata: {
              source: "swappie-html-snapshot",
              source_url: item.source_url,
              source_sku: item.sku,
              source_file_sha256: manifest.source_file_sha256,
              extracted_at: manifest.extracted_at,
              available_storages: item.available_storages_gb.map(
                (value) => `${value} GB`,
              ),
              available_colors: item.available_colors,
              available_batteries: supplements.battery_options,
              available_conditions: supplements.condition_options,
              display_size_inches: specification.display_size_inches,
              rear_camera_count: specification.rear_camera_count,
              apple_model_specs: specification,
              card_colors: item.card_colors,
              display_spec_source_url: specification.display_source_url,
              camera_spec_source_url: specification.camera_source_url,
              release_year: item.release_year,
              source_release_year_code: item.source_release_year_code,
            },
            options: [{ title: "Storage", values: [storage] }],
            variants: [
              {
                title: storage,
                sku: item.sku,
                manage_inventory: true,
                allow_backorder: false,
                options: { Storage: storage },
                prices: [{ currency_code: "eur", amount: item.price_eur }],
              },
            ],
          },
        ],
      },
    });
    created += 1;
    console.log(`Imported ${item.model}: ${item.sku}`);
  }

  console.log(
    `Swappie import complete: ${created} created, ${updated} updated, ${skipped} skipped`,
  );
}
