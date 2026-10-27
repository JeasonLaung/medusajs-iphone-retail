-- Explicit empty-draft transition. Existing rows are never discarded or hidden.
-- A failed precondition stops before RENAME; the runner requires reconciliation.
CREATE TABLE catalog_schema_transition_guard (
  id TINYINT PRIMARY KEY CHECK (id=1),
  preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO catalog_schema_transition_guard(id,preconditions_met)
SELECT 1,(NOT EXISTS(SELECT 1 FROM `catalog_products` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `catalog_variants` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `catalog_prices` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `catalog_categories` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `catalog_category_products` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `catalog_collections` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM `commerce_regions` LIMIT 1)
 AND NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('catalog_images','catalog_option_values','catalog_options','catalog_product_channels','catalog_product_option_values','catalog_product_options','catalog_product_shipping_profiles','catalog_product_tags','catalog_product_types','catalog_tags','catalog_variant_images','catalog_variant_inventory_items','catalog_variant_options','catalog_variant_price_sets','commerce_channel_locations','commerce_countries','commerce_currencies','commerce_region_payment_providers','commerce_sales_channels','commerce_store_currencies','commerce_store_locales','commerce_stores','fulfillment_option_price_sets','fulfillment_option_types','fulfillment_providers','fulfillment_service_zones','fulfillment_sets','fulfillment_shipping_options','fulfillment_shipping_profiles','inventory_items','inventory_location_addresses','inventory_locations','legacy_0001_catalog_categories','legacy_0001_catalog_category_products','legacy_0001_catalog_collections','legacy_0001_catalog_prices','legacy_0001_catalog_products','legacy_0001_catalog_variants','legacy_0001_commerce_regions','payment_providers','pricing_preferences','pricing_price_list_rules','pricing_price_lists','pricing_price_rules','pricing_price_sets','pricing_prices')));
RENAME TABLE `catalog_products` TO `legacy_0001_catalog_products`,
 `catalog_variants` TO `legacy_0001_catalog_variants`,
 `catalog_prices` TO `legacy_0001_catalog_prices`,
 `catalog_categories` TO `legacy_0001_catalog_categories`,
 `catalog_category_products` TO `legacy_0001_catalog_category_products`,
 `catalog_collections` TO `legacy_0001_catalog_collections`,
 `commerce_regions` TO `legacy_0001_commerce_regions`;

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

CREATE TABLE `commerce_currencies` (
  `code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `symbol` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `symbol_native` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `decimal_digits` INT NOT NULL,
  `rounding` DECIMAL(38,18) NOT NULL,
  `raw_rounding` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f609def6421011137a2cbd9a` CHECK (`raw_rounding` IS NULL OR JSON_VALID(`raw_rounding`)),
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_providers` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_1dd40b99adf2cb7d8ad31e61` CHECK (`is_enabled` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_sets` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_cf952c1f8f057b463e1d2ecd` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_images` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_2d8453a98f51418577b1cfff` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `rank` INT NOT NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `inventory_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `sku` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `origin_country` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `hs_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `mid_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `material` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `weight` INT NULL,
  `length` INT NULL,
  `height` INT NULL,
  `width` INT NULL,
  `requires_shipping` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_4bcfb27aea742d4b528bc9b4` CHECK (`requires_shipping` IN (0,1)),
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `thumbnail` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_7bc4096a64dc9a5b533a3826` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `unit_of_measure` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_providers` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_9b2f3fc4dccc6c0787acb36d` CHECK (`is_enabled` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_prices` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `price_set_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_3e4df6d03a77adc49e1495e2` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `rules_count` INT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `price_list_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `min_quantity` DECIMAL(38,18) NULL,
  `max_quantity` DECIMAL(38,18) NULL,
  `raw_min_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_1a13f69ab5a254f4c85c00bf` CHECK (`raw_min_quantity` IS NULL OR JSON_VALID(`raw_min_quantity`)),
  `raw_max_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_b89d5621c1c35adef5f0136b` CHECK (`raw_max_quantity` IS NULL OR JSON_VALID(`raw_max_quantity`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_price_lists` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `starts_at` DATETIME(6) NULL,
  `ends_at` DATETIME(6) NULL,
  `rules_count` INT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_3c2899e46ab9e41d155e1ba4` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_a27bd2b27fcd218f7a4413a3` CHECK (`status` IN ('active','draft')),
  CONSTRAINT `ck_source_4e85fc348c0df9a4fcdb725d` CHECK (`type` IN ('sale','override'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_price_list_rules` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `price_list_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_83d9d9bcce419d398756ce90` CHECK (`value` IS NULL OR JSON_VALID(`value`)),
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_preferences` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_a60bb7a159dc6e347c0d5f25` CHECK (`is_tax_inclusive` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_price_rules` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `priority` INT NOT NULL,
  `price_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `operator` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_source_e55fe9a6de4a62a2c6fbc4c8` CHECK (`operator` IN ('gte','lte','gt','lt','eq')),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `pricing_price_sets` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_products` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `handle` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subtitle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_giftcard` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_bead0f96c8121abfa6376cf0` CHECK (`is_giftcard` IN (0,1)),
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `thumbnail` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `weight` DOUBLE NULL,
  `length` DOUBLE NULL,
  `height` DOUBLE NULL,
  `width` DOUBLE NULL,
  `origin_country` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `hs_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `mid_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `material` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `type_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `discountable` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_b371ee38c517600eff307731` CHECK (`discountable` IN (0,1)),
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_17a13eaf7d76993eba53dbef` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_2e5dc2ae1f93d65492afbcdd` CHECK (`status` IN ('draft','proposed','published','rejected'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_categories` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `handle` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `mpath` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_active` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_df267f3aa42802a83efe236a` CHECK (`is_active` IN (0,1)),
  `is_internal` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_77a01dba754b3c1640ab17fb` CHECK (`is_internal` IN (0,1)),
  `rank` INT NOT NULL,
  `parent_category_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_65d661a0e076e5f0277c5ecd` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_category_products` (
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `category_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`product_id`,`category_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_collections` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `handle` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_771f72e0bb86c6ab47f9ab3b` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_options` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6c318243bb539a69ca459e7b` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `is_exclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_74d89010d50b90b19da6d188` CHECK (`is_exclusive` IN (0,1)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_option_values` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6a3851f7536778dfcae43bc3` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `rank` INT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_options` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_option_values` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_product_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_option_value_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_channels` (
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `sales_channel_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`product_id`,`sales_channel_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_shipping_profiles` (
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shipping_profile_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`product_id`,`shipping_profile_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_tags` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_62177a79c020833dd28779e8` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_tags` (
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `tag_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`product_id`,`tag_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_product_types` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_02dcb4f353be6124849c9262` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_variants` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `sku` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `barcode` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `ean` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `upc` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `allow_backorder` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_84a1c71407e8ea22dbb02cc4` CHECK (`allow_backorder` IN (0,1)),
  `manage_inventory` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_7874ecbf66cc500e6e17cea5` CHECK (`manage_inventory` IN (0,1)),
  `hs_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `origin_country` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `mid_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `material` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `weight` DOUBLE NULL,
  `length` DOUBLE NULL,
  `height` DOUBLE NULL,
  `width` DOUBLE NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_1ada89a4ec8fa12cbc76989d` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `variant_rank` INT NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `thumbnail` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_variant_inventory_items` (
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `inventory_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `required_quantity` DECIMAL(38,18) NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`variant_id`,`inventory_item_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_variant_options` (
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `option_value_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`variant_id`,`option_value_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_variant_price_sets` (
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `price_set_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`variant_id`,`price_set_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `catalog_variant_images` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `image_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_regions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_1f2c16c61abb926a1d93dbbc` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `automatic_taxes` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_3d8c4182a73d3b9e3851f814` CHECK (`automatic_taxes` IN (0,1)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_countries` (
  `iso_2` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `iso_3` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `num_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `display_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_55e88eee2afd4f6b2551a35c` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`iso_2`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_region_payment_providers` (
  `region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`region_id`,`payment_provider_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_sales_channels` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_disabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_9e620e5e63801b99ea541277` CHECK (`is_disabled` IN (0,1)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_0dfbd166a293d5ad362e27f7` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_channel_locations` (
  `sales_channel_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `stock_location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`sales_channel_id`,`stock_location_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_service_zones` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_df01d77352334fab93ecd76b` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `fulfillment_set_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_shipping_options` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `price_type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `service_zone_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shipping_profile_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_699ff095f69fbc132f6a9a76` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_355d64adf4791a0092b65af3` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `shipping_option_type_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_a6f287a1dafe024e33f1b787` CHECK (`price_type` IN ('calculated','flat'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_option_price_sets` (
  `shipping_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `price_set_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`shipping_option_id`,`price_set_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_option_types` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `label` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_shipping_profiles` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_31bf520e8b2b56c879c48389` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `inventory_locations` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_0dae7fbcf03880bf56974ab6` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `inventory_location_addresses` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `address_1` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `address_2` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `company` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `city` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `province` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `postal_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_104b7866fdcd56e08ecc1d03` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_stores` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `default_sales_channel_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `default_region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `default_location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_55a77a432e9355db1cea9ea3` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_store_currencies` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_default` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_e935a17c56a627aab5663998` CHECK (`is_default` IN (0,1)),
  `store_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `commerce_store_locales` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `locale_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `store_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE INDEX `idx_source_b1ae793e383fd7a74bd3c754` ON `fulfillment_providers` (`deleted_at`);

CREATE INDEX `idx_source_9b6717e127c71559c2136868` ON `fulfillment_sets` (`deleted_at`);

CREATE INDEX `idx_source_2f79595ddad9cdaffaea865b` ON `catalog_images` (`deleted_at`);

CREATE INDEX `idx_source_e25f8c01a191fa1868eb692f` ON `catalog_images` (`product_id`);

CREATE INDEX `idx_source_7dbdff864f562a575f8c2aeb` ON `catalog_images` (`rank`);

CREATE INDEX `idx_source_709c498700d8717e317617f6` ON `catalog_images` (`rank`,`product_id`);

CREATE INDEX `idx_source_80688e9495be62b72b91c07a` ON `catalog_images` (`url`(128));

CREATE INDEX `idx_source_0fda8d102114103ade7980cc` ON `catalog_images` (`url`(128),`rank`,`product_id`);

CREATE INDEX `idx_source_90baac2acc18aa3e2f64465a` ON `inventory_items` (`deleted_at`);

ALTER TABLE `inventory_items` ADD COLUMN `active_key_64bad780756ba441a34edf3c` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_64bad780756ba441a34edf3c` ON `inventory_items` (`sku`,`active_key_64bad780756ba441a34edf3c`);

CREATE INDEX `idx_source_72bd56ea6b19f1483ae5c109` ON `payment_providers` (`deleted_at`);

CREATE INDEX `idx_source_7e5f619702be034a691c96c5` ON `pricing_prices` (`currency_code`);

CREATE INDEX `idx_source_4d3970d44764243c44d8a6cf` ON `pricing_prices` (`deleted_at`);

CREATE INDEX `idx_source_dba160da1bd09f780ccb6e8d` ON `pricing_prices` (`price_list_id`);

CREATE INDEX `idx_source_30d343425aec3229c7b65345` ON `pricing_prices` (`price_set_id`);

CREATE INDEX `idx_source_495d3c71dd4b7b7141cc88bd` ON `pricing_price_lists` (`deleted_at`);

CREATE INDEX `idx_source_036c94a002d72bd9cb32ab85` ON `pricing_price_lists` (`id`,`status`(128),`starts_at`,`ends_at`);

CREATE INDEX `idx_source_028ca9fca2914647889fbd1f` ON `pricing_price_list_rules` (`attribute`(128));

CREATE INDEX `idx_source_bcc925508102918804fcc62c` ON `pricing_price_list_rules` (`deleted_at`);

CREATE INDEX `idx_source_ba9d124588be74109f9971dc` ON `pricing_price_list_rules` (`price_list_id`);

CREATE INDEX `idx_source_f009f582b51ab41d35decce2` ON `pricing_preferences` (`deleted_at`);

CREATE INDEX `idx_source_62e605046dd32f64846503ce` ON `pricing_price_rules` (`attribute`(128));

CREATE INDEX `idx_source_0f83a700ce1e2cdd25dc7b25` ON `pricing_price_rules` (`attribute`(128),`value`(128));

CREATE INDEX `idx_source_a4e468e75d25befd06b65761` ON `pricing_price_rules` (`attribute`(128),`value`(128),`price_id`);

CREATE INDEX `idx_source_8204550f1652a9b7c9276021` ON `pricing_price_rules` (`deleted_at`);

CREATE INDEX `idx_source_ef23d63997efcfee4213030f` ON `pricing_price_rules` (`operator`(128));

CREATE INDEX `idx_source_419f2c5610d5342e4e3c7d33` ON `pricing_price_rules` (`operator`(128),`value`(128));

CREATE INDEX `idx_source_3447badbbcdf875789acad15` ON `pricing_price_rules` (`price_id`);

CREATE INDEX `idx_source_1be06040047ea66981663063` ON `pricing_price_sets` (`deleted_at`);

CREATE INDEX `idx_source_d3595294fb22aeb39d9473b6` ON `catalog_products` (`collection_id`);

CREATE INDEX `idx_source_864b2b372ae80a23e9d660bc` ON `catalog_products` (`deleted_at`);

ALTER TABLE `catalog_products` ADD COLUMN `active_key_c5b6f5d7aadc0d64e1d840b3` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_c5b6f5d7aadc0d64e1d840b3` ON `catalog_products` (`handle`,`active_key_c5b6f5d7aadc0d64e1d840b3`);

CREATE INDEX `idx_source_565a7e7d59a30549de8f9071` ON `catalog_products` (`status`(128));

CREATE INDEX `idx_source_1215bd26dc5885900e0f71ba` ON `catalog_products` (`type_id`);

ALTER TABLE `catalog_categories` ADD COLUMN `active_key_6bc49d200e1ce6d594e5b3f6` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_6bc49d200e1ce6d594e5b3f6` ON `catalog_categories` (`handle`,`active_key_6bc49d200e1ce6d594e5b3f6`);

CREATE INDEX `idx_source_d44736371247392131c1c215` ON `catalog_categories` (`parent_category_id`);

CREATE INDEX `idx_source_08b5e5995e8117dbea0e43b6` ON `catalog_categories` (`mpath`(128));

ALTER TABLE `catalog_collections` ADD COLUMN `active_key_7e77265756e2b1abab35ab89` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_7e77265756e2b1abab35ab89` ON `catalog_collections` (`handle`,`active_key_7e77265756e2b1abab35ab89`);

CREATE INDEX `idx_source_2daaa595567b834ef3afe502` ON `catalog_collections` (`deleted_at`);

CREATE INDEX `idx_source_627384cac94c85fbfeabed65` ON `catalog_options` (`deleted_at`);

CREATE INDEX `idx_source_22ee406b2e2c36553c119466` ON `catalog_option_values` (`deleted_at`);

CREATE INDEX `idx_source_081769f4809e40724b2af870` ON `catalog_option_values` (`option_id`);

CREATE INDEX `idx_source_90ef6c37842d835341441e79` ON `catalog_product_options` (`deleted_at`);

CREATE INDEX `idx_source_e81e52edc1e2fda492f583ec` ON `catalog_product_options` (`product_id`);

CREATE INDEX `idx_source_09b4f8baee880e8704f828ec` ON `catalog_product_options` (`product_option_id`);

CREATE INDEX `idx_source_c543899cd7fa30a464a307cb` ON `catalog_product_option_values` (`deleted_at`);

CREATE INDEX `idx_source_5572a9d81da90af85587053c` ON `catalog_product_option_values` (`product_option_value_id`);

CREATE INDEX `idx_source_8d949b0f8bd2484c3cec033e` ON `catalog_product_option_values` (`product_product_option_id`);

CREATE INDEX `idx_source_052b87de3935c4a829d7d424` ON `catalog_product_option_values` (`product_product_option_id`);

CREATE INDEX `idx_source_417b8e7513bf89fce6ea4b02` ON `catalog_product_channels` (`deleted_at`);

CREATE INDEX `idx_source_8c730d2f9f0c7a968fd7712a` ON `catalog_product_channels` (`id`);

CREATE INDEX `idx_source_86f891abe960ce460cf04f11` ON `catalog_product_channels` (`product_id`);

CREATE INDEX `idx_source_269c56bb7abd1fad87fe8a1f` ON `catalog_product_channels` (`sales_channel_id`);

CREATE INDEX `idx_source_4a852d7fff4019f7f8b3d014` ON `catalog_product_shipping_profiles` (`deleted_at`);

CREATE INDEX `idx_source_f20efc074bef535e385031bc` ON `catalog_product_shipping_profiles` (`id`);

CREATE INDEX `idx_source_1fa6b294f0ac3b98bb530b2f` ON `catalog_product_shipping_profiles` (`product_id`);

CREATE INDEX `idx_source_fff35697b8f0811b99544227` ON `catalog_product_shipping_profiles` (`shipping_profile_id`);

CREATE INDEX `idx_source_e9db27988e0c7b7b54ee5167` ON `catalog_tags` (`deleted_at`);

CREATE INDEX `idx_source_2545dd9fc1c013ea5d4a6b31` ON `catalog_product_types` (`deleted_at`);

ALTER TABLE `catalog_variants` ADD COLUMN `active_key_e6afc5865574fa33281e3560` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_e6afc5865574fa33281e3560` ON `catalog_variants` (`barcode`,`active_key_e6afc5865574fa33281e3560`);

CREATE INDEX `idx_source_bfb2c123c6a94a97e40ba4bd` ON `catalog_variants` (`deleted_at`);

ALTER TABLE `catalog_variants` ADD COLUMN `active_key_901e2edf61a83a9712c90da7` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_901e2edf61a83a9712c90da7` ON `catalog_variants` (`ean`,`active_key_901e2edf61a83a9712c90da7`);

CREATE INDEX `idx_source_4906a3e5235e5222684ba99c` ON `catalog_variants` (`id`,`product_id`);

CREATE INDEX `idx_source_d39157f815f76605e998abc6` ON `catalog_variants` (`product_id`);

ALTER TABLE `catalog_variants` ADD COLUMN `active_key_788f5109fe1a6aa9abda8005` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_788f5109fe1a6aa9abda8005` ON `catalog_variants` (`sku`,`active_key_788f5109fe1a6aa9abda8005`);

ALTER TABLE `catalog_variants` ADD COLUMN `active_key_1a3e447f2483af18a126c893` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_1a3e447f2483af18a126c893` ON `catalog_variants` (`upc`,`active_key_1a3e447f2483af18a126c893`);

CREATE INDEX `idx_source_1b537cce47764766a1218a01` ON `catalog_variant_inventory_items` (`deleted_at`);

CREATE INDEX `idx_source_7d919ed3cf027765d5ecf771` ON `catalog_variant_inventory_items` (`id`);

CREATE INDEX `idx_source_1db3495633784d7b747901bd` ON `catalog_variant_inventory_items` (`inventory_item_id`);

CREATE INDEX `idx_source_4ef7197036d4ace8bf1437f7` ON `catalog_variant_inventory_items` (`variant_id`);

CREATE INDEX `idx_source_4cefb5473d5ad9aac7ddf2a4` ON `catalog_variant_price_sets` (`deleted_at`);

CREATE INDEX `idx_source_e0d31354f6ebcdb7c87f196e` ON `catalog_variant_price_sets` (`id`);

CREATE INDEX `idx_source_2a9accebea2fc760e0dc92e9` ON `catalog_variant_price_sets` (`price_set_id`);

CREATE INDEX `idx_source_0bb7b731932dde8b58a08225` ON `catalog_variant_price_sets` (`variant_id`);

CREATE INDEX `idx_source_9dffa4a6678797e66718304b` ON `catalog_variant_images` (`deleted_at`);

CREATE INDEX `idx_source_2ad06a6c4a286f403ead4fb7` ON `catalog_variant_images` (`image_id`);

CREATE INDEX `idx_source_1ffa2807f73458a9b517aaec` ON `catalog_variant_images` (`variant_id`);

CREATE INDEX `idx_source_69c907f08cafde8ea39357e7` ON `commerce_regions` (`deleted_at`);

CREATE INDEX `idx_source_910f7b721be25210ed3460f5` ON `commerce_countries` (`deleted_at`);

CREATE INDEX `idx_source_6e31dce258c02e3afb91a51e` ON `commerce_countries` (`region_id`);

CREATE UNIQUE INDEX `idx_source_87f6ec973e5899911b745351` ON `commerce_countries` (`region_id`,`iso_2`);

CREATE INDEX `idx_source_89adbaf0bd564b891f763e42` ON `commerce_region_payment_providers` (`deleted_at`);

CREATE INDEX `idx_source_b576bbd8709286c1b4260e90` ON `commerce_region_payment_providers` (`id`);

CREATE INDEX `idx_source_9423a567d8b5625d65bd8124` ON `commerce_region_payment_providers` (`payment_provider_id`);

CREATE INDEX `idx_source_f888006a65f5df8fe891cd18` ON `commerce_region_payment_providers` (`region_id`);

CREATE INDEX `idx_source_09b96c6170a705e97717ef0c` ON `commerce_sales_channels` (`deleted_at`);

CREATE INDEX `idx_source_dd9214d1d15a03092f34b031` ON `commerce_channel_locations` (`deleted_at`);

CREATE INDEX `idx_source_0180848ce31babf95d45edcf` ON `commerce_channel_locations` (`id`);

CREATE INDEX `idx_source_5fc2c9522b02d08c7d79b12c` ON `commerce_channel_locations` (`sales_channel_id`);

CREATE INDEX `idx_source_666e89cf505ace7e1a145d25` ON `commerce_channel_locations` (`stock_location_id`);

CREATE INDEX `idx_source_3973e2e36016d4a28b3c6de3` ON `fulfillment_service_zones` (`deleted_at`);

CREATE INDEX `idx_source_464c04176c3c2a1ccb0988e3` ON `fulfillment_service_zones` (`fulfillment_set_id`);

CREATE INDEX `idx_source_874da305825f601e7a6b6335` ON `fulfillment_shipping_options` (`deleted_at`);

CREATE INDEX `idx_source_05038cdf3462a562c3ab2198` ON `fulfillment_shipping_options` (`provider_id`);

CREATE INDEX `idx_source_85b99497a107c9b43bec55dd` ON `fulfillment_shipping_options` (`service_zone_id`);

CREATE INDEX `idx_source_5b94669b9ec10811a85e4bc8` ON `fulfillment_shipping_options` (`shipping_option_type_id`);

CREATE INDEX `idx_source_178812f75a6b8b400550c699` ON `fulfillment_shipping_options` (`shipping_profile_id`);

CREATE INDEX `idx_source_f99ed1a1b2f0990c5a5a2c66` ON `fulfillment_option_price_sets` (`deleted_at`);

CREATE INDEX `idx_source_d5f7be081856ab3ace4e2fad` ON `fulfillment_option_price_sets` (`id`);

CREATE INDEX `idx_source_01936f64920a8650b05447ad` ON `fulfillment_option_price_sets` (`price_set_id`);

CREATE INDEX `idx_source_111fde5db9e1116cbb81fbf3` ON `fulfillment_option_price_sets` (`shipping_option_id`);

CREATE INDEX `idx_source_e048e4d16a50602fe1b704ec` ON `fulfillment_option_types` (`deleted_at`);

CREATE INDEX `idx_source_17dba00a2216ad90bc5292ce` ON `fulfillment_shipping_profiles` (`deleted_at`);

ALTER TABLE `inventory_locations` ADD COLUMN `active_key_bef42b8230bd7fee3111608f` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_bef42b8230bd7fee3111608f` ON `inventory_locations` (`address_id`,`active_key_bef42b8230bd7fee3111608f`);

CREATE INDEX `idx_source_98c23c985874c7a4aa9c32c1` ON `inventory_locations` (`deleted_at`);

CREATE INDEX `idx_source_f447aa67274c51e7f77f2bfd` ON `inventory_location_addresses` (`deleted_at`);

CREATE INDEX `idx_source_d6d224b4e6f1cb0f952d675d` ON `commerce_stores` (`deleted_at`);

CREATE INDEX `idx_source_9382f8392acea3dd9be75a49` ON `commerce_store_currencies` (`deleted_at`);

CREATE INDEX `idx_source_b60851e363642766d95fe0bb` ON `commerce_store_currencies` (`store_id`);

CREATE INDEX `idx_source_c0bf2d85f14c8ed004a2275b` ON `commerce_store_locales` (`deleted_at`);

CREATE INDEX `idx_source_9d5e220a8ac99d43306ea24a` ON `commerce_store_locales` (`store_id`);

ALTER TABLE `catalog_images` ADD CONSTRAINT `fk_source_ede5b1bdc2ef9a86887ae7a8` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `pricing_prices` ADD CONSTRAINT `fk_source_12fbaf804bb3248e0b5d6e63` FOREIGN KEY (`price_list_id`) REFERENCES `pricing_price_lists` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `pricing_prices` ADD CONSTRAINT `fk_source_8b439dcff9678783fa1d3f4c` FOREIGN KEY (`price_set_id`) REFERENCES `pricing_price_sets` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `pricing_price_list_rules` ADD CONSTRAINT `fk_source_762f562955f3f85d224b96d4` FOREIGN KEY (`price_list_id`) REFERENCES `pricing_price_lists` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `pricing_price_rules` ADD CONSTRAINT `fk_source_a82858b737aeacb6d6184059` FOREIGN KEY (`price_id`) REFERENCES `pricing_prices` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_products` ADD CONSTRAINT `fk_source_d90a958b33c2f7ad4abcdc5a` FOREIGN KEY (`collection_id`) REFERENCES `catalog_collections` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `catalog_products` ADD CONSTRAINT `fk_source_441facce0de669199ce16b68` FOREIGN KEY (`type_id`) REFERENCES `catalog_product_types` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `catalog_categories` ADD CONSTRAINT `fk_source_3f74b14bdb7f7da55949a51c` FOREIGN KEY (`parent_category_id`) REFERENCES `catalog_categories` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_category_products` ADD CONSTRAINT `fk_source_488fa7b504f510a0acbbe417` FOREIGN KEY (`category_id`) REFERENCES `catalog_categories` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_category_products` ADD CONSTRAINT `fk_source_7e8777afc2248447be545e76` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_option_values` ADD CONSTRAINT `fk_source_b218844c8b72217c41ccb323` FOREIGN KEY (`option_id`) REFERENCES `catalog_options` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_options` ADD CONSTRAINT `fk_source_bc7a77fc66338ae356648548` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_options` ADD CONSTRAINT `fk_source_d044054a9093dfdb9b3b6e4a` FOREIGN KEY (`product_option_id`) REFERENCES `catalog_options` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_option_values` ADD CONSTRAINT `fk_source_7ed5b0f7e112a6bc15b1742f` FOREIGN KEY (`product_option_value_id`) REFERENCES `catalog_option_values` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_option_values` ADD CONSTRAINT `fk_source_fb4f0153261a6804966904fc` FOREIGN KEY (`product_product_option_id`) REFERENCES `catalog_product_options` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_tags` ADD CONSTRAINT `fk_source_983b29b036e9ded2a4060929` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_product_tags` ADD CONSTRAINT `fk_source_83324b12b811121f39c5541d` FOREIGN KEY (`tag_id`) REFERENCES `catalog_tags` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_variants` ADD CONSTRAINT `fk_source_dcadba4585b73996e99b17dc` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_variant_options` ADD CONSTRAINT `fk_source_5ac9c2179469a322a60b05e9` FOREIGN KEY (`option_value_id`) REFERENCES `catalog_option_values` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_variant_options` ADD CONSTRAINT `fk_source_2e0da62cd05d1fc4568ce3fe` FOREIGN KEY (`variant_id`) REFERENCES `catalog_variants` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_variant_images` ADD CONSTRAINT `fk_source_67527edce1483d26e9d07925` FOREIGN KEY (`image_id`) REFERENCES `catalog_images` (`id`) ON DELETE CASCADE;

ALTER TABLE `commerce_countries` ADD CONSTRAINT `fk_source_c62c42b3b2ba01d0a4e12093` FOREIGN KEY (`region_id`) REFERENCES `commerce_regions` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_service_zones` ADD CONSTRAINT `fk_source_41ee77d2322d75a7dc4fb7bb` FOREIGN KEY (`fulfillment_set_id`) REFERENCES `fulfillment_sets` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_b5ee90c53fbc6c39f5311fc7` FOREIGN KEY (`provider_id`) REFERENCES `fulfillment_providers` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_e078d8dff4a0b585a9295cf2` FOREIGN KEY (`service_zone_id`) REFERENCES `fulfillment_service_zones` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_8b9cb28f9981c97bbec4ffc9` FOREIGN KEY (`shipping_option_type_id`) REFERENCES `fulfillment_option_types` (`id`) ON UPDATE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_d2edc156282c291dec6ddf56` FOREIGN KEY (`shipping_profile_id`) REFERENCES `fulfillment_shipping_profiles` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `inventory_locations` ADD CONSTRAINT `fk_source_a6e6f417f5b505788abeb94a` FOREIGN KEY (`address_id`) REFERENCES `inventory_location_addresses` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `commerce_store_currencies` ADD CONSTRAINT `fk_source_8bc7cc47e67aea2cb767b374` FOREIGN KEY (`store_id`) REFERENCES `commerce_stores` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `commerce_store_locales` ADD CONSTRAINT `fk_source_37766b489ef7d0ad92df27c1` FOREIGN KEY (`store_id`) REFERENCES `commerce_stores` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

-- Read schema only. Commands must implement their own permissions, unique-key
-- registry, concurrency and audit before any later write capability is enabled.
CREATE TABLE catalog_runtime_schema (
  id TINYINT PRIMARY KEY CHECK (id=1),
  schema_version INT NOT NULL CHECK (schema_version=1),
  model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
  writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO catalog_runtime_schema VALUES (1,1,'35c4fcfc359b01eb553f41ac7a3aa184c87f9c86216639dfb9e495c482d3d76d',TRUE,FALSE);
