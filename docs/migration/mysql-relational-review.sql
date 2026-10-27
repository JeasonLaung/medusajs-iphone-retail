-- REVIEW ONLY: not a production migration. Unique indexes/defaults/identity holds remain.
-- No IF NOT EXISTS: a pre-existing table must not hide incompatible schema.
-- All imported fields are explicit; importer must supply required values; no source nextval/trigger/default runs.
SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';
SET SESSION time_zone='+00:00';

CREATE TABLE `payment_account_holders` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `after_sales_requests` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reason` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `items` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_8c2b2809de2b9e53b9f622a9` CHECK (`items` IS NULL OR JSON_VALID(`items`)),
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_2a195d3ca295ef224869964a` CHECK (`status` IN ('pending','processing','completed','rejected')),
  CONSTRAINT `ck_source_3b4e133a3c42c680c3a38de4` CHECK (`type` IN ('return','exchange','repair'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_buy_rules` (
  `application_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_rule_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`application_method_id`,`promotion_rule_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_target_rules` (
  `application_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_rule_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`application_method_id`,`promotion_rule_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `buyback_device_links` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `request_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `device_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `buyback_photos` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `request_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `photo_type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `uploaded_at` DATETIME(6) NOT NULL,
  `storage_key` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `mime_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `review_status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `review_note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_e8a6af704f772e0fcd1c3a51` CHECK (`photo_type` IN ('front','back','edges','damage','packaging','sealed')),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_3f627fd2484cb29bc3746d6e` CHECK (`review_status` IN ('pending','approved','retake','expired'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `buyback_prices` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `capacity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_33705ff7e6bb5e05e9a7dd46` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `pricing_rules` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_719eb3c908c29bf6013fedab` CHECK (`pricing_rules` IS NULL OR JSON_VALID(`pricing_rules`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `buyback_requests` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `submission_key` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `contact_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `return_address` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_fa49436604148f2fb9720b03` CHECK (`return_address` IS NULL OR JSON_VALID(`return_address`)),
  `items` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_8e845e32d0ed12ccaf59556c` CHECK (`items` IS NULL OR JSON_VALID(`items`)),
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference_amount` DECIMAL(38,18) NULL,
  `final_amount` DECIMAL(38,18) NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shipping_instructions` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `tracking_number` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `return_tracking` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `review_note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `history` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_b81f48387a7b8fad5e40181c` CHECK (`history` IS NULL OR JSON_VALID(`history`)),
  `closed_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `raw_reference_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_4e87f393ab0943a8d83eb7fd` CHECK (`raw_reference_amount` IS NULL OR JSON_VALID(`raw_reference_amount`)),
  `raw_final_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_bb389dca70dbbe162689e57b` CHECK (`raw_final_amount` IS NULL OR JSON_VALID(`raw_final_amount`)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_292946da7176b6e5379e895d` CHECK (`status` IN ('awaiting_photos','photo_review','ready_to_ship','in_transit','received','final_offer','accepted','paid','return_requested','returning','returned','canceled'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_captures` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_3fd075dff4f6b32c103512c7` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `payment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_ff0fa3fca206a2958c1f18d0` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `carts` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `sales_channel_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shipping_address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `billing_address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_7b3a193fc32b91e36e0d9f77` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `completed_at` DATETIME(6) NULL,
  `locale` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_addresses` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `company` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `first_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `last_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_1` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_2` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `city` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `province` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `postal_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_e9765a96272c13e17351d056` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_line_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subtitle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `thumbnail` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `quantity` INT NOT NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_subtitle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_collection` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_handle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_sku` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_barcode` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_option_values` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_2979530bdc9fa310a25da97b` CHECK (`variant_option_values` IS NULL OR JSON_VALID(`variant_option_values`)),
  `requires_shipping` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_81e1528fb5c180cd7bfcbb42` CHECK (`requires_shipping` IN (0,1)),
  `is_discountable` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_aa462ac0b33864942fb7ac81` CHECK (`is_discountable` IN (0,1)),
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_53fdc4aa34bd105f9cf90753` CHECK (`is_tax_inclusive` IN (0,1)),
  `compare_at_unit_price` DECIMAL(38,18) NULL,
  `raw_compare_at_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_5f5a521828bf35343a43a5fe` CHECK (`raw_compare_at_unit_price` IS NULL OR JSON_VALID(`raw_compare_at_unit_price`)),
  `unit_price` DECIMAL(38,18) NOT NULL,
  `raw_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_8b1d15dc4c6f02d608b71b8f` CHECK (`raw_unit_price` IS NULL OR JSON_VALID(`raw_unit_price`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_cf6b2cad659d4b3abb7d1494` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `product_type_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_custom_price` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_1aea6518e8b0c4581732aedc` CHECK (`is_custom_price` IN (0,1)),
  `is_giftcard` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_6540c8ceefa188c6042854b2` CHECK (`is_giftcard` IN (0,1)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_c7748739ac7c0c4682a06e91` CHECK (`unit_price` >= 0)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_line_adjustments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_965720562866d9308c30ac3b` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_ef74d888e8de2b6ea84ea70b` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_c9f960948b07e2a0c0108104` CHECK (`is_tax_inclusive` IN (0,1)),
  CONSTRAINT `ck_source_5ed11927ce72a1b5072fc6d6` CHECK (`amount` >= 0),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_line_tax_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `tax_rate_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rate` DECIMAL(38,18) NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_cf9023c455e8657690dcc646` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_78a803e514fe2f65314c933a` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_payment_collections` (
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`cart_id`,`payment_collection_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_promotions` (
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`cart_id`,`promotion_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_shipping_methods` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_aa4967862d0d7915b70803fe` CHECK (`description` IS NULL OR JSON_VALID(`description`)),
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_10e342a4f518860a8ed71930` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_abe3a60bbc1524d1487d6cba` CHECK (`is_tax_inclusive` IN (0,1)),
  `shipping_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_29c1cc8a8254947be617f81b` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_f9702b51c03b01779e4d56dd` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_078627cbc86fd408d79c1c5e` CHECK (`amount` >= 0),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_shipping_adjustments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f6f69413ec074e062ae8ea5c` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_e5ee84ff57cf0f45bafc0074` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `shipping_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_shipping_tax_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `tax_rate_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rate` DECIMAL(38,18) NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_eae51a2736415fce6242137b` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `shipping_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_73cba985a313da0d0953fd7e` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `cart_credit_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reference_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_600209e7a6d29c159bd10e19` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_4e4e9def67c0121b305ae751` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

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

CREATE TABLE `historical_customer_subjects` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `company_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `first_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `last_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `legacy_had_account` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_524288007e07724b84c02378` CHECK (`legacy_had_account` IN (0,1)),
  `metadata` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `historical_customer_groups` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGBLOB NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `historical_customer_memberships` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_group_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `customer_notices` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `event_key` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `kind` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `resource_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `action` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `read_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `devices` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `serial_number` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `imei` VARCHAR(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `grade` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `battery_health` INT NULL,
  `cost_cents` BIGINT NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `bin` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `notes` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `state` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `imei2` VARCHAR(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `category` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `photos` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_a3e8bebdf2877560587bdd6f` CHECK (`photos` IS NULL OR JSON_VALID(`photos`)),
  `eid` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_source_2c3914d32902148554c251e2` CHECK (`category` IN ('sale','returned','recycled')),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_5706369df60332000bd84607` CHECK (`state` IN ('pending','inspection','available','reserved','packed','shipped','repair','repair_ready','retired','supplier_returned'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `device_assignments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `device_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `after_sales_request_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `replacement_of` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `tracking_number` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_d79c0a288a81adee3a0b2390` CHECK (`status` IN ('reserved','packed','shipped','returned','canceled'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `device_events` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `device_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `actor_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `action` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_034e9f2ac56df949683f472c` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `device_sources` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_925f4bb35e4397123202f120` CHECK (`type` IN ('self','supplier'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `device_supply_commitments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` INT NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `device_variant_tracking` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `inventory_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `packed_at` DATETIME(6) NULL,
  `shipped_at` DATETIME(6) NULL,
  `delivered_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_72f773cc05b5f6a811e88597` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `shipping_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_3d01079f91f3747b92ce46dc` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `delivery_address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `marked_shipped_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `requires_shipping` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_1606415b2119910a733b89b3` CHECK (`requires_shipping` IN (0,1)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_addresses` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `company` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `first_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `last_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_1` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_2` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `city` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `province` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `postal_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_0dda6d92d7ab870c19eeea3e` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `sku` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `barcode` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_fb1cd3c81e2c5d7350fb3032` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  `line_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `inventory_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `fulfillment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_labels` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `tracking_number` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `tracking_url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `label_url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
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

CREATE TABLE `fulfillment_geo_zones` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `province_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `city` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `service_zone_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `postal_expression` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_744d6e5da5f5752b05119908` CHECK (`postal_expression` IS NULL OR JSON_VALID(`postal_expression`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_1d30b1cd218aecdd812a85ec` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_d51f1b760e9a537b2079ba24` CHECK (`type` IN ('country','province','city','zip'))
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

CREATE TABLE `inventory_levels` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `inventory_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `stocked_quantity` DECIMAL(38,18) NOT NULL,
  `reserved_quantity` DECIMAL(38,18) NOT NULL,
  `incoming_quantity` DECIMAL(38,18) NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_2e05d937f2ccfcca03731484` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `raw_stocked_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_95478a81feaf23732754d7dd` CHECK (`raw_stocked_quantity` IS NULL OR JSON_VALID(`raw_stocked_quantity`)),
  `raw_reserved_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_b5e30df30b7ade51dfd77231` CHECK (`raw_reserved_quantity` IS NULL OR JSON_VALID(`raw_reserved_quantity`)),
  `raw_incoming_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_4fdcc1208ab487eb2232086c` CHECK (`raw_incoming_quantity` IS NULL OR JSON_VALID(`raw_incoming_quantity`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_location_providers` (
  `stock_location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`stock_location_id`,`fulfillment_provider_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `fulfillment_location_sets` (
  `stock_location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_set_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`stock_location_id`,`fulfillment_set_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_accounts` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reply_to` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_bd21aadfb34be930b7a36c32` CHECK (`enabled` IN (0,1)),
  `api_key` LONGBLOB NOT NULL,
  `webhook_secret` LONGBLOB NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_campaigns` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `account_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subject` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `text_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `html_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_188e48a7960d582a9bbfb827` CHECK (`status` IN ('draft','queued','sending','paused','completed'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_contacts` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subscribed` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_9a8f84a92ba7fc75aa131cf6` CHECK (`subscribed` IN (0,1)),
  `consent_source` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `consent_at` DATETIME(6) NULL,
  `suppressed` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_11b375b2ce37cc4492c79e72` CHECK (`suppressed` IN (0,1)),
  `unsubscribe_token` LONGBLOB NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_deliveries` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `campaign_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `contact_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `attempts` INT NOT NULL,
  `error` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `sent_at` DATETIME(6) NULL,
  `opened_at` DATETIME(6) NULL,
  `clicked_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `first_attempt_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_events` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `event_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `account_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `occurred_at` DATETIME(6) NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_messages` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `account_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `folder` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `previous_folder` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `direction` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `from_address` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `to` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_dd9258bdd5ab747239eb5a76` CHECK (`to` IS NULL OR JSON_VALID(`to`)),
  `cc` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_a085ef71eee3efd9ff43dd75` CHECK (`cc` IS NULL OR JSON_VALID(`cc`)),
  `bcc` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_8f063ec4c6d8fcdb07425324` CHECK (`bcc` IS NULL OR JSON_VALID(`bcc`)),
  `subject` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `text_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `html_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attachments` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f4dceb3ba5f946284bcf1664` CHECK (`attachments` IS NULL OR JSON_VALID(`attachments`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `message_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `in_reply_to` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reply_to` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_5c65d5761d9218610f960af4` CHECK (`reply_to` IS NULL OR JSON_VALID(`reply_to`)),
  `read` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_26e2219bd96d6a83fe883f03` CHECK (`read` IN (0,1)),
  `starred` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_b4a2c18b5985a209fae5e296` CHECK (`starred` IN (0,1)),
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `error` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attempts` INT NOT NULL,
  `sent_at` DATETIME(6) NULL,
  `received_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `first_attempt_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_9de15aa8b5b4c706ff3239f7` CHECK (`direction` IN ('inbound','outbound')),
  CONSTRAINT `ck_source_c78ea847f9899a54887821be` CHECK (`folder` IN ('inbox','draft','outbox','sent','archive','trash')),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mail_templates` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subject` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `text_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `html_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `mollie_configs` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `environment` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `api_key` LONGBLOB NOT NULL,
  `redirect_url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_f58330ca0cd91655957c4ba7` CHECK (`enabled` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_256506cf2981c504b87e08c6` CHECK (`environment` IN ('test','production')),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `historical_notifications` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `to` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `channel` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `template` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGBLOB NULL,
  `trigger_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `resource_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `resource_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `receiver_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `original_notification_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `idempotency_key` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `from` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `provider_data` LONGBLOB NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_903b9fb57127698575759788` CHECK (`status` IN ('pending','success','failure'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `notification_providers` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `handle` VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_bbbf0d40b3ec32f758d62fd8` CHECK (`is_enabled` IN (0,1)),
  `channels` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_906c4580b123a0dae03eae26` CHECK (`channels` IS NULL OR JSON_VALID(`channels`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `orders` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `display_id` INT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `version` INT NOT NULL,
  `sales_channel_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_draft_order` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_067437aa308e8bdf12972c14` CHECK (`is_draft_order` IN (0,1)),
  `email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shipping_address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `billing_address_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `no_notification` BOOLEAN NULL,
  CONSTRAINT `ck_bool_48b52a9f5363b79c4114ffd1` CHECK (`no_notification` IN (0,1)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_dc843e9284c0706ed9ccf820` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `custom_display_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `locale` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_addresses` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `company` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `first_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `last_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_1` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `address_2` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `city` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `province` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `postal_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `phone` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_9cadce48826555b9dc34321f` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_carts` (
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `cart_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`order_id`,`cart_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_changes` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `version` INT NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `internal_note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `requested_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `requested_at` DATETIME(6) NULL,
  `confirmed_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `confirmed_at` DATETIME(6) NULL,
  `declined_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `declined_reason` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_aaef8bcd0fa455df05d19fbd` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `declined_at` DATETIME(6) NULL,
  `canceled_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `canceled_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `change_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `deleted_at` DATETIME(6) NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `carry_over_promotions` BOOLEAN NULL,
  CONSTRAINT `ck_bool_b9f76441bea63c08a35000f5` CHECK (`carry_over_promotions` IN (0,1)),
  `no_notification` BOOLEAN NULL,
  CONSTRAINT `ck_bool_c4c0a1e102af2856b57440c3` CHECK (`no_notification` IN (0,1)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_8dbc717c7fbbcb41ddbd02de` CHECK (`status` IN ('confirmed','declined','requested','pending','canceled'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_change_actions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `version` INT NULL,
  `ordering` BIGINT NOT NULL,
  `order_change_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reference_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `action` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `details` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_ed653a831ce59841bf5291fe` CHECK (`details` IS NULL OR JSON_VALID(`details`)),
  `amount` DECIMAL(38,18) NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_a65cc011ce54d28cd1dfb99d` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `internal_note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `applied` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_309aaef7dd7d9da46b09327c` CHECK (`applied` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_claims` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `order_version` INT NOT NULL,
  `display_id` INT NOT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `no_notification` BOOLEAN NULL,
  CONSTRAINT `ck_bool_da40b0bea77c9aaa29e1fe44` CHECK (`no_notification` IN (0,1)),
  `refund_amount` DECIMAL(38,18) NULL,
  `raw_refund_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_3472c517f95844829cb14a06` CHECK (`raw_refund_amount` IS NULL OR JSON_VALID(`raw_refund_amount`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_5a6a16b7411cdcd9572e12d3` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_claim_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_additional_item` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_9c01553ec6bfbba89e0612f2` CHECK (`is_additional_item` IN (0,1)),
  `reason` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_ac53b25d9c718183e1f44577` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  `note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_9957d9c20b62f95b87ca2b18` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_claim_item_images` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `claim_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_ac6e1e6a1319ce29667c086e` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_credit_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reference_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f708a715530287e58a48063f` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_81ed0aaf74e673e0a79fa5e3` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `version` INT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_exchanges` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `order_version` INT NOT NULL,
  `display_id` INT NOT NULL,
  `no_notification` BOOLEAN NULL,
  CONSTRAINT `ck_bool_000f658af9173a19d2f745e8` CHECK (`no_notification` IN (0,1)),
  `allow_backorder` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_bc11d64a714be932c22a8307` CHECK (`allow_backorder` IN (0,1)),
  `difference_due` DECIMAL(38,18) NULL,
  `raw_difference_due` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_42e2fcee9edefc0f9616125a` CHECK (`raw_difference_due` IS NULL OR JSON_VALID(`raw_difference_due`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_98d4b7cc6dc1a9c768716af2` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_exchange_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_7e4d16ccae5093555b7d6665` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  `note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_1c2a8116184b6c0f24acc1c4` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_fulfillments` (
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`order_id`,`fulfillment_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `version` INT NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_7961e4b7f62485433a6bf566` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  `fulfilled_quantity` DECIMAL(38,18) NOT NULL,
  `raw_fulfilled_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_d21a9601cb390240ea5776a2` CHECK (`raw_fulfilled_quantity` IS NULL OR JSON_VALID(`raw_fulfilled_quantity`)),
  `shipped_quantity` DECIMAL(38,18) NOT NULL,
  `raw_shipped_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_7767df9c694ee05921a2b158` CHECK (`raw_shipped_quantity` IS NULL OR JSON_VALID(`raw_shipped_quantity`)),
  `return_requested_quantity` DECIMAL(38,18) NOT NULL,
  `raw_return_requested_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_fc888e909117706b36e69082` CHECK (`raw_return_requested_quantity` IS NULL OR JSON_VALID(`raw_return_requested_quantity`)),
  `return_received_quantity` DECIMAL(38,18) NOT NULL,
  `raw_return_received_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_9a9abd0093d661c24b4281b7` CHECK (`raw_return_received_quantity` IS NULL OR JSON_VALID(`raw_return_received_quantity`)),
  `return_dismissed_quantity` DECIMAL(38,18) NOT NULL,
  `raw_return_dismissed_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_441ecadc9b7f67652eb4921b` CHECK (`raw_return_dismissed_quantity` IS NULL OR JSON_VALID(`raw_return_dismissed_quantity`)),
  `written_off_quantity` DECIMAL(38,18) NOT NULL,
  `raw_written_off_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_5db3adf0e111d28efd3f0989` CHECK (`raw_written_off_quantity` IS NULL OR JSON_VALID(`raw_written_off_quantity`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_eef1326bcc8739c9a1039459` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `delivered_quantity` DECIMAL(38,18) NOT NULL,
  `raw_delivered_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_125aae06cf9caeebfbe90588` CHECK (`raw_delivered_quantity` IS NULL OR JSON_VALID(`raw_delivered_quantity`)),
  `unit_price` DECIMAL(38,18) NULL,
  `raw_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_044b776e19c60182ae4da6b5` CHECK (`raw_unit_price` IS NULL OR JSON_VALID(`raw_unit_price`)),
  `compare_at_unit_price` DECIMAL(38,18) NULL,
  `raw_compare_at_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_a91199d14894367d75576f67` CHECK (`raw_compare_at_unit_price` IS NULL OR JSON_VALID(`raw_compare_at_unit_price`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_line_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `totals_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `subtitle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `thumbnail` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_subtitle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_collection` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `product_handle` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_sku` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_barcode` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `variant_option_values` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_fceec7795f904f5ed7a2edd1` CHECK (`variant_option_values` IS NULL OR JSON_VALID(`variant_option_values`)),
  `requires_shipping` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_ab1dc793c00405a7de17e5c0` CHECK (`requires_shipping` IN (0,1)),
  `is_discountable` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_ac3a90b459e12f37e14d8ad8` CHECK (`is_discountable` IN (0,1)),
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_2293afc54f7e3f9ceb5f5742` CHECK (`is_tax_inclusive` IN (0,1)),
  `compare_at_unit_price` DECIMAL(38,18) NULL,
  `raw_compare_at_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_33df46d268b1cc13d1bf19e0` CHECK (`raw_compare_at_unit_price` IS NULL OR JSON_VALID(`raw_compare_at_unit_price`)),
  `unit_price` DECIMAL(38,18) NOT NULL,
  `raw_unit_price` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_c6b815195277d8a5fbfeaef8` CHECK (`raw_unit_price` IS NULL OR JSON_VALID(`raw_unit_price`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_e8b5fbe9683542c2e0780f98` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `is_custom_price` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_02c2abcac0c21e8f23d302dc` CHECK (`is_custom_price` IN (0,1)),
  `product_type_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_giftcard` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_93e1aaa23172056acaa822b3` CHECK (`is_giftcard` IN (0,1)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_line_adjustments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_183fa3e56a65fd1ba9ac4a89` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_a3bc0e13a0cf356f4e162f2e` CHECK (`is_tax_inclusive` IN (0,1)),
  `version` INT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_line_tax_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `tax_rate_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rate` DECIMAL(38,18) NOT NULL,
  `raw_rate` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_690b8f795327d347aa145270` CHECK (`raw_rate` IS NULL OR JSON_VALID(`raw_rate`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_7a15bb6518a8bcb68944122e` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_49f48d034768560e83f88fce` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_number_clock` (
  `id` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_e39167ce39522244d34419c0` CHECK (`id` IN (0,1)),
  `last_tick` BIGINT NOT NULL,
  CONSTRAINT `ck_source_3b02b7e261b7f4c58c3819a7` CHECK (`id` = 1),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_payment_collections` (
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`order_id`,`payment_collection_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_promotions` (
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`order_id`,`promotion_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_shipping_versions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `version` INT NOT NULL,
  `shipping_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_shipping_methods` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6adc4508b989a17923275381` CHECK (`description` IS NULL OR JSON_VALID(`description`)),
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_d6dd3fa589803d68f4eee50a` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_e7a0ffc13526119e4e927e8e` CHECK (`is_tax_inclusive` IN (0,1)),
  `shipping_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_cab9048f49190b4be27fd656` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_83ec19641ccc29209144f3f7` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `is_custom_amount` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_96b84472ecd6448ace337e13` CHECK (`is_custom_amount` IN (0,1)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_shipping_adjustments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_46b395866dbe51a3b694a433` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `shipping_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `version` INT NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_shipping_tax_lines` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `tax_rate_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rate` DECIMAL(38,18) NOT NULL,
  `raw_rate` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_837e38d9fa5be937a4cc22f2` CHECK (`raw_rate` IS NULL OR JSON_VALID(`raw_rate`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `shipping_method_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_b7ab5a7ebb8f84438729f4a9` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_21717ba0aabbc79c2df50ae9` CHECK (`data` IS NULL OR JSON_VALID(`data`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_summaries` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `version` INT NOT NULL,
  `totals` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_eb8aede87d5c7f3414d13061` CHECK (`totals` IS NULL OR JSON_VALID(`totals`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `order_transactions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `version` INT NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_e56f8e242128af398d3ed2b6` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `reference_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payments` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_c953c40f93a927c85e053c69` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `captured_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `payment_collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_session_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_collections` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_545c81f3dee5613107d3739a` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `authorized_amount` DECIMAL(38,18) NULL,
  `raw_authorized_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_abff8a821ad049755ec6f453` CHECK (`raw_authorized_amount` IS NULL OR JSON_VALID(`raw_authorized_amount`)),
  `captured_amount` DECIMAL(38,18) NULL,
  `raw_captured_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_9e2391d2d841def472a32583` CHECK (`raw_captured_amount` IS NULL OR JSON_VALID(`raw_captured_amount`)),
  `refunded_amount` DECIMAL(38,18) NULL,
  `raw_refunded_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_4ef35e5f7c223b151fd2b06b` CHECK (`raw_refunded_amount` IS NULL OR JSON_VALID(`raw_refunded_amount`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `completed_at` DATETIME(6) NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_838ed02b3b2c52256d1aaabe` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_865b5814d1938b5b0d4ac557` CHECK (`status` IN ('not_paid','awaiting','authorized','partially_authorized','canceled','failed','partially_captured','completed'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_collection_providers` (
  `payment_collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `payment_provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`payment_collection_id`,`payment_provider_id`)
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

CREATE TABLE `payment_sessions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_c98214877cedc3a211539059` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `context` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `authorized_at` DATETIME(6) NULL,
  `payment_collection_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_14e29f75d358e530003dcc99` CHECK (`status` IN ('authorized','captured','pending','requires_more','error','canceled','pending_authorization'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `paypal_connections` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `environment` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `shared_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `auth_code` LONGBLOB NULL,
  `seller_client_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `seller_client_secret` LONGBLOB NULL,
  `app_access_token` LONGBLOB NULL,
  `app_access_token_expires_at` DATETIME(6) NULL,
  `metadata` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `seller_email` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `seller_merchant_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `paypal_legacy_metrics` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `data` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `paypal_settings` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `data` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `paypal_webhook_events` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `event_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `event_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `resource_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `payload` LONGBLOB NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `event_version` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `transmission_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `transmission_time` DATETIME(6) NULL,
  `status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attempt_count` INT NOT NULL,
  `next_retry_at` DATETIME(6) NULL,
  `processed_at` DATETIME(6) NULL,
  `last_error` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
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

CREATE TABLE `product_reviews` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `product_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `customer_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `author` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rating` INT NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `content` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `verified_purchase` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_5c48baf7cc4e43d3218af26a` CHECK (`verified_purchase` IN (0,1)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_1057dd3df88a21e4dcd97f7f` CHECK (`status` IN ('pending','approved','rejected'))
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

CREATE TABLE `promotions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `campaign_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `is_automatic` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_ec9dd1220844bb2a49836e76` CHECK (`is_automatic` IN (0,1)),
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_tax_inclusive` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_2571bfb9adcf8e06f4d434cc` CHECK (`is_tax_inclusive` IN (0,1)),
  `limit` INT NULL,
  `used` INT NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6e840b12dd86b3095ef0bb58` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_17b801d225e58d4348130fe5` CHECK (`status` IN ('draft','active','inactive')),
  CONSTRAINT `ck_source_d2ac9f98ce7e4c41961b27d7` CHECK (`type` IN ('standard','buyget'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_application_methods` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` DECIMAL(38,18) NULL,
  `raw_value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_8d491d2179dd22d2c3ba9ec7` CHECK (`raw_value` IS NULL OR JSON_VALID(`raw_value`)),
  `max_quantity` INT NULL,
  `apply_to_quantity` INT NULL,
  `buy_rules_min_quantity` INT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `target_type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `allocation` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_source_b538cba3834d2244fbc91cc2` CHECK (`allocation` IN ('each','across','once')),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_40bbc83d1dda00bf22b7634f` CHECK (`target_type` IN ('order','shipping_methods','items')),
  CONSTRAINT `ck_source_1368f4ad3f5f48528f4d43d1` CHECK (`type` IN ('fixed','percentage'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_campaigns` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `campaign_identifier` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `starts_at` DATETIME(6) NULL,
  `ends_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_campaign_budgets` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `campaign_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `limit` DECIMAL(38,18) NULL,
  `raw_limit` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_e8b6d614c375e16be3f7a79b` CHECK (`raw_limit` IS NULL OR JSON_VALID(`raw_limit`)),
  `used` DECIMAL(38,18) NOT NULL,
  `raw_used` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f81215252804aea10819d997` CHECK (`raw_used` IS NULL OR JSON_VALID(`raw_used`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `currency_code` VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_6dcbdd3cde67ba40afbdcbff` CHECK (`type` IN ('spend','usage','use_by_attribute','spend_by_attribute'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_budget_usages` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attribute_value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `used` DECIMAL(38,18) NOT NULL,
  `budget_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `raw_used` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_d6ed91a52a1ea060c30dd03c` CHECK (`raw_used` IS NULL OR JSON_VALID(`raw_used`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_order_rules` (
  `promotion_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_rule_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  PRIMARY KEY (`promotion_id`,`promotion_rule_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_rules` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `operator` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_3187ccc745f87dfb2f9cb8dd` CHECK (`operator` IN ('gte','lte','gt','lt','eq','ne','in')),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `promotion_rule_values` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `promotion_rule_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_refunds` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `amount` DECIMAL(38,18) NOT NULL,
  `raw_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_9f1863428fd2eaa21c00ec0f` CHECK (`raw_amount` IS NULL OR JSON_VALID(`raw_amount`)),
  `payment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6fe87c2cc335e8e82e622371` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `refund_reason_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `payment_refund_reasons` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `label` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_0afe0bbb2049517427b45057` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
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

CREATE TABLE `inventory_reservations` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `line_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `external_id` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_f644523d7980402d893b14e7` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `inventory_item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `allow_backorder` BOOLEAN NULL,
  CONSTRAINT `ck_bool_c01c43c2038ebd602eaaa7c5` CHECK (`allow_backorder` IN (0,1)),
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_6bbd3837821231947afb9775` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `returns` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `order_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `claim_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `exchange_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `order_version` INT NOT NULL,
  `display_id` INT NOT NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `no_notification` BOOLEAN NULL,
  CONSTRAINT `ck_bool_6df0ec9643d30332e96f269c` CHECK (`no_notification` IN (0,1)),
  `refund_amount` DECIMAL(38,18) NULL,
  `raw_refund_amount` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_b5b975a027e443b60b7a4a2f` CHECK (`raw_refund_amount` IS NULL OR JSON_VALID(`raw_refund_amount`)),
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_2976612fc3a27cceee56a067` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `received_at` DATETIME(6) NULL,
  `canceled_at` DATETIME(6) NULL,
  `location_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `requested_at` DATETIME(6) NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `return_fulfillments` (
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `fulfillment_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`return_id`,`fulfillment_id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `return_items` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `return_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reason_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `item_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `quantity` DECIMAL(38,18) NOT NULL,
  `raw_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_caba8d761008e2a52bc45c81` CHECK (`raw_quantity` IS NULL OR JSON_VALID(`raw_quantity`)),
  `received_quantity` DECIMAL(38,18) NOT NULL,
  `raw_received_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_506cc0d69c0854ebb58f85e1` CHECK (`raw_received_quantity` IS NULL OR JSON_VALID(`raw_received_quantity`)),
  `note` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_8f408473c8308769af3c0c70` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  `damaged_quantity` DECIMAL(38,18) NOT NULL,
  `raw_damaged_quantity` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_e5b188ba45b85d4fe4bcd89c` CHECK (`raw_damaged_quantity` IS NULL OR JSON_VALID(`raw_damaged_quantity`)),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `return_reasons` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `label` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_c77bb133337b13c3db76d299` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `parent_return_reason_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
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

CREATE TABLE `seo_crawler_visits` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `crawler_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `request_path` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status_code` INT NOT NULL,
  `client_ip_hash` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `occurred_at` DATETIME(6) NOT NULL,
  `event_hash` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_evidence` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `run_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `evidence_type` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_249cd72d3aefb100adeb799d` CHECK (`evidence_type` IN ('provider_citation','answer_link')),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_monitors` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `monitor_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `brand_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `query_text` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `interval_minutes` INT NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_fb68a2edcbeeefeb75597331` CHECK (`enabled` IN (0,1)),
  `last_run_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_e505c948d15fb8c58eab311d` CHECK (`provider` IN ('chatgpt','deepseek'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_pages` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `path` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `title` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `description` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `canonical` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `og_image` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `hreflang` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_50d3a2d88ff270f1032a2f17` CHECK (`hreflang` IS NULL OR JSON_VALID(`hreflang`)),
  `structured_data` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_5822206ca5c3b6283bbe98e8` CHECK (`structured_data` IS NULL OR JSON_VALID(`structured_data`)),
  `robots` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_providers` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `api_key` LONGBLOB NOT NULL,
  `base_url` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `model` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `wire_api` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_63c2ff46074fd43aad1367fc` CHECK (`provider` IN ('chatgpt','deepseek')),
  CONSTRAINT `ck_source_cacb1f5740125cc71b6236e0` CHECK (`wire_api` IN ('responses','chat','chat_input'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_runs` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `monitor_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `model` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `prompt_text` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `answer_text` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `brand_mentioned` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_b4b44504e3d99d1c08ac829e` CHECK (`brand_mentioned` IN (0,1)),
  `citation_count` INT NOT NULL,
  `started_at` DATETIME(6) NOT NULL,
  `finished_at` DATETIME(6) NULL,
  `error_message` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_65352d1a2f29f9a79176fff3` CHECK (`status` IN ('running','completed','failed'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_settings` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `key` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGBLOB NOT NULL,
  `is_secret` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_319e2a791fd4e4e4d93a7d36` CHECK (`is_secret` IN (0,1)),
  `group` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_sources` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `file_path` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `interval_minutes` INT NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_0c26872fedc414bd78e08593` CHECK (`enabled` IN (0,1)),
  `collect_ai_crawlers` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_3ba3ebb1157163ed359ae767` CHECK (`collect_ai_crawlers` IN (0,1)),
  `collect_traffic` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_7a57390ea1058db17b1cca46` CHECK (`collect_traffic` IN (0,1)),
  `read_offset` DECIMAL(38,18) NOT NULL,
  `read_lines` INT NOT NULL,
  `file_fingerprint` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `matched_lines` INT NOT NULL,
  `traffic_matched_lines` INT NOT NULL,
  `last_scanned_at` DATETIME(6) NULL,
  `last_error` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `raw_read_offset` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_f303334947a7a364e3401051` CHECK (`raw_read_offset` IS NULL OR JSON_VALID(`raw_read_offset`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_submissions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `success` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_4595306415e8ebf22ff2ecae` CHECK (`success` IN (0,1)),
  `submitted_count` INT NOT NULL,
  `http_status` INT NOT NULL,
  `urls` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_48a79d60c8affdbc75cd30cf` CHECK (`urls` IS NULL OR JSON_VALID(`urls`)),
  `response_headers` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_74683706116ecefd74be1bb4` CHECK (`response_headers` IS NULL OR JSON_VALID(`response_headers`)),
  `response_body` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `error` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `submitted_at` DATETIME(6) NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `seo_traffic_visits` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `log_source_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `platform` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `source` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `medium` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `campaign` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `click_id_type` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `matched_by` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `landing_path` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `referrer_host` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `status_code` INT NOT NULL,
  `client_ip` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `location` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `occurred_at` DATETIME(6) NOT NULL,
  `event_hash` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
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

CREATE TABLE `fulfillment_option_rules` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `attribute` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `operator` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `value` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_b27b16cdc43a4748ea6af0cf` CHECK (`value` IS NULL OR JSON_VALID(`value`)),
  `shipping_option_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_c726ad7e6888377ed60be562` CHECK (`operator` IN ('in','eq','ne','gt','gte','lt','lte','nin')),
  PRIMARY KEY (`id`)
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

CREATE TABLE `sku_number_counter` (
  `id` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_8c6c28c0f51184bc974d9427` CHECK (`id` IN (0,1)),
  `last_id` BIGINT NOT NULL,
  CONSTRAINT `ck_source_c8a511701cd8c9cb443183e6` CHECK (`id` = 1),
  CONSTRAINT `ck_source_fe7715ff4683a88b5cbb278f` CHECK (`last_id` >= 0),
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

CREATE TABLE `tax_providers` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_50692c36d215b7ea61e5a878` CHECK (`is_enabled` IN (0,1)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `tax_rates` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `rate` DECIMAL(38,18) NULL,
  `code` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `is_default` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_124648bd915d17271aff5042` CHECK (`is_default` IN (0,1)),
  `is_combinable` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_72b2570a607046fcae154ab9` CHECK (`is_combinable` IN (0,1)),
  `tax_region_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_3e7a48963e90169bdfe4d8be` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `tax_rate_rules` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `tax_rate_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `reference` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_d8c307d7166a84796226f388` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `tax_regions` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `country_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `province_code` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `parent_id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `metadata` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  CONSTRAINT `ck_json_5c19402e4f8f72b21a88f3dc` CHECK (`metadata` IS NULL OR JSON_VALID(`metadata`)),
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `created_by` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_ee32576cd65a306727779522` CHECK (`parent_id` IS NULL OR `province_code` IS NOT NULL),
  CONSTRAINT `ck_source_1c0ada82939ad130f7638f54` CHECK (`parent_id` IS NULL OR `provider_id` IS NULL),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE TABLE `webhook_endpoints` (
  `id` VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `name` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `provider` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `url` LONGBLOB NOT NULL,
  `secret` LONGBLOB NOT NULL,
  `token` LONGBLOB NOT NULL,
  `keyword` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  CONSTRAINT `ck_bool_ef950680a855bb70649e348c` CHECK (`enabled` IN (0,1)),
  `events` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  CONSTRAINT `ck_json_c147a604ff2b831009652cdb` CHECK (`events` IS NULL OR JSON_VALID(`events`)),
  `language` VARCHAR(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `last_status` LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
  `last_sent_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  `deleted_at` DATETIME(6) NULL,
  CONSTRAINT `ck_source_d4bb2df941f4cdfce524fecb` CHECK (`language` IN ('zh','en')),
  PRIMARY KEY (`id`),
  CONSTRAINT `ck_source_3050abe7d5a5f12382774b6f` CHECK (`provider` IN ('feishu','wecom','dingtalk','slack','mattermost','generic'))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;

CREATE INDEX `idx_source_f6a03a9c77a0a750f40867b4` ON `payment_account_holders` (`deleted_at`);

ALTER TABLE `after_sales_requests` ADD COLUMN `active_key_5d6020ceb086aaef0f924314` TINYINT GENERATED ALWAYS AS (IF(`status` IN ('pending','processing') AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_5d6020ceb086aaef0f924314` ON `after_sales_requests` (`order_id`,`active_key_5d6020ceb086aaef0f924314`);

CREATE INDEX `idx_source_5331134eceb3b5ece762ff7c` ON `after_sales_requests` (`deleted_at`);

CREATE INDEX `idx_source_d646959007d5ca9c5fcba3c9` ON `after_sales_requests` (`order_id`);

CREATE INDEX `idx_source_03d849d3a36aafe7997a9ba3` ON `buyback_device_links` (`deleted_at`);

ALTER TABLE `buyback_device_links` ADD COLUMN `active_key_a992cbf854fabeef9a0641cd` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_a992cbf854fabeef9a0641cd` ON `buyback_device_links` (`device_id`,`active_key_a992cbf854fabeef9a0641cd`);

ALTER TABLE `buyback_device_links` ADD COLUMN `active_key_ddc9e6d8547e79f2f412ce3a` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ddc9e6d8547e79f2f412ce3a` ON `buyback_device_links` (`item_id`,`active_key_ddc9e6d8547e79f2f412ce3a`);

CREATE INDEX `idx_source_2a4debcc1bb4a084b22ee36f` ON `buyback_photos` (`deleted_at`);

CREATE INDEX `idx_source_d276e5fc9d112f56bd7b38a4` ON `buyback_photos` (`request_id`,`item_id`);

CREATE INDEX `idx_source_d18a2f41c60fa4b0721fd63a` ON `buyback_prices` (`deleted_at`);

CREATE INDEX `idx_source_983d457b432e7cfa9b9cdd95` ON `buyback_requests` (`customer_id`,`created_at`);

CREATE INDEX `idx_source_89b5283d68a444023776cf7f` ON `buyback_requests` (`deleted_at`);

CREATE INDEX `idx_source_f72dc889790cd323b3b0866b` ON `buyback_requests` (`status`,`created_at`);

CREATE INDEX `idx_source_cc67305c791f4262650bfe94` ON `payment_captures` (`deleted_at`);

CREATE INDEX `idx_source_52b7020667a98d464a344fe8` ON `payment_captures` (`payment_id`);

CREATE INDEX `idx_source_afe30d8df4d4783030ef3227` ON `carts` (`billing_address_id`);

CREATE INDEX `idx_source_cc2d5fa6f9f0fbf6ef27e343` ON `carts` (`currency_code`);

CREATE INDEX `idx_source_e37988ad00abfb338e3bbd45` ON `carts` (`customer_id`);

CREATE INDEX `idx_source_7c32e671380db62a0efcc203` ON `carts` (`deleted_at`);

CREATE INDEX `idx_source_9820892ef7845e43c4389c30` ON `carts` (`region_id`);

CREATE INDEX `idx_source_a31f158bf9b3b15a7ca65bc0` ON `carts` (`sales_channel_id`);

CREATE INDEX `idx_source_fa4e32ebb18bf36a98960d53` ON `carts` (`shipping_address_id`);

CREATE INDEX `idx_source_69f4323efa5a2f54960ef933` ON `cart_addresses` (`deleted_at`);

CREATE INDEX `idx_source_315bb559911a147cb8ad9608` ON `cart_line_items` (`cart_id`);

CREATE INDEX `idx_source_6073f516948b2dcbb8035974` ON `cart_line_items` (`deleted_at`);

CREATE INDEX `idx_source_13ef1cd397d9edb91e4f318f` ON `cart_line_items` (`product_id`);

CREATE INDEX `idx_source_0f85549db9ae740d4c87f2d1` ON `cart_line_items` (`variant_id`);

CREATE INDEX `idx_source_2857b95b80ca4a668f83c5b2` ON `cart_line_adjustments` (`deleted_at`);

CREATE INDEX `idx_source_205ed0b29f1bd44948096349` ON `cart_line_adjustments` (`item_id`);

CREATE INDEX `idx_source_36d43f548f98e42e456e5988` ON `cart_line_adjustments` (`promotion_id`);

CREATE INDEX `idx_source_a40c10f643a554dc5420506b` ON `cart_line_tax_lines` (`deleted_at`);

CREATE INDEX `idx_source_fbb6729b786caf5e08df64ee` ON `cart_line_tax_lines` (`item_id`);

CREATE INDEX `idx_source_9dedf1990433945f5377b689` ON `cart_line_tax_lines` (`tax_rate_id`);

CREATE INDEX `idx_source_04c652218f508db5e0c70b4a` ON `cart_payment_collections` (`cart_id`);

CREATE INDEX `idx_source_6c7bf287c7c89e60e453d306` ON `cart_payment_collections` (`deleted_at`);

CREATE INDEX `idx_source_4359f0c4d286fcf1319fcf12` ON `cart_payment_collections` (`id`);

CREATE INDEX `idx_source_0e13994b0cf843d5fb9b779c` ON `cart_payment_collections` (`payment_collection_id`);

CREATE INDEX `idx_source_7e6313c1a3046494f80e6a52` ON `cart_promotions` (`cart_id`);

CREATE INDEX `idx_source_36065e50b67e6549cc8906f7` ON `cart_promotions` (`deleted_at`);

CREATE INDEX `idx_source_82386a9a8af3f019981311db` ON `cart_promotions` (`id`);

CREATE INDEX `idx_source_93fc35b323456e5087c5947b` ON `cart_promotions` (`promotion_id`);

CREATE INDEX `idx_source_6e6c25279f48f0eddb142e47` ON `cart_shipping_methods` (`cart_id`);

CREATE INDEX `idx_source_d05d9b04feb00e5328bec7b1` ON `cart_shipping_methods` (`deleted_at`);

CREATE INDEX `idx_source_14c04b9a5bc2b21d9b10670f` ON `cart_shipping_methods` (`shipping_option_id`);

CREATE INDEX `idx_source_2fad0350c183274f4df1708f` ON `cart_shipping_adjustments` (`deleted_at`);

CREATE INDEX `idx_source_f6c72851105278a01b0b894b` ON `cart_shipping_adjustments` (`shipping_method_id`);

CREATE INDEX `idx_source_3e79869f6a87bc981ad0cb7b` ON `cart_shipping_adjustments` (`promotion_id`);

CREATE INDEX `idx_source_087dd7c70c9e46388f85f282` ON `cart_shipping_tax_lines` (`deleted_at`);

CREATE INDEX `idx_source_cb5862298039c6122daefc06` ON `cart_shipping_tax_lines` (`shipping_method_id`);

CREATE INDEX `idx_source_95407e89813bf58ff16da2b0` ON `cart_shipping_tax_lines` (`tax_rate_id`);

CREATE INDEX `idx_source_97589992a42ab3e5ae350ccc` ON `cart_credit_lines` (`reference`(128),`reference_id`);

CREATE INDEX `idx_source_f44ed5d828721b484944f221` ON `cart_credit_lines` (`cart_id`);

CREATE INDEX `idx_source_6deafc7e0bad1887c5b1a5c3` ON `cart_credit_lines` (`deleted_at`);

CREATE INDEX `idx_source_57fd489466a8ccdb6bb6df87` ON `historical_customer_subjects` (`deleted_at`);

CREATE INDEX `idx_source_fd061842f7cf4ff812f71d62` ON `historical_customer_groups` (`deleted_at`);

CREATE INDEX `idx_source_8cfe8ea5bf4aa7164d1ecfd9` ON `historical_customer_memberships` (`customer_group_id`);

CREATE INDEX `idx_source_cfe7f43b4fa11b9a288893ba` ON `historical_customer_memberships` (`customer_id`);

CREATE INDEX `idx_source_ea456e9934b5a20127243e09` ON `historical_customer_memberships` (`deleted_at`);

CREATE INDEX `idx_source_b2e27da801f4718598de81df` ON `customer_notices` (`customer_id`,`created_at`);

CREATE INDEX `idx_source_edcf2ebea54c5d10288cbc49` ON `customer_notices` (`customer_id`,`read_at`);

CREATE INDEX `idx_source_527f67713fa82fab1460519c` ON `customer_notices` (`deleted_at`);

CREATE INDEX `idx_source_61687548440acfb87a201cf2` ON `devices` (`deleted_at`);

ALTER TABLE `devices` ADD COLUMN `active_key_fe4207e0a763f39cc5e0b1e9` TINYINT GENERATED ALWAYS AS (IF(`eid` IS NOT NULL AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_fe4207e0a763f39cc5e0b1e9` ON `devices` (`eid`,`active_key_fe4207e0a763f39cc5e0b1e9`);

ALTER TABLE `devices` ADD COLUMN `active_key_1ce9e7a2e7d84c2896b77595` TINYINT GENERATED ALWAYS AS (IF(`imei2` IS NOT NULL AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_1ce9e7a2e7d84c2896b77595` ON `devices` (`imei2`,`active_key_1ce9e7a2e7d84c2896b77595`);

ALTER TABLE `devices` ADD COLUMN `active_key_6d51059f132f40a249dd515c` TINYINT GENERATED ALWAYS AS (IF(`imei` IS NOT NULL AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_6d51059f132f40a249dd515c` ON `devices` (`imei`,`active_key_6d51059f132f40a249dd515c`);

ALTER TABLE `devices` ADD COLUMN `active_key_c9bd86c8979d2b5914ead3fb` TINYINT GENERATED ALWAYS AS (IF(`serial_number` IS NOT NULL AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_c9bd86c8979d2b5914ead3fb` ON `devices` (`serial_number`,`active_key_c9bd86c8979d2b5914ead3fb`);

CREATE INDEX `idx_source_36c763273943e92f9c9e2087` ON `devices` (`variant_id`,`location_id`,`state`);

CREATE INDEX `idx_source_c35231deda3b3f9db639eb74` ON `device_assignments` (`deleted_at`);

ALTER TABLE `device_assignments` ADD COLUMN `active_key_56867ee56515053f055967d0` TINYINT GENERATED ALWAYS AS (IF(`status` IN ('reserved','packed','shipped') AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_56867ee56515053f055967d0` ON `device_assignments` (`device_id`,`active_key_56867ee56515053f055967d0`);

CREATE INDEX `idx_source_2860aee3b5133a461b7f51e3` ON `device_assignments` (`order_id`,`item_id`);

CREATE INDEX `idx_source_c9729e4880fff943b5261f10` ON `device_events` (`deleted_at`);

CREATE INDEX `idx_source_83925f6769ec1b1425101390` ON `device_events` (`device_id`);

CREATE INDEX `idx_source_b822b869d3b5728e0883a7f5` ON `device_events` (`order_id`);

CREATE INDEX `idx_source_11a22feb957cfb1827e03295` ON `device_sources` (`deleted_at`);

ALTER TABLE `device_sources` ADD COLUMN `active_key_2a35c787dd96c557d1f04c69` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_2a35c787dd96c557d1f04c69` ON `device_sources` (`location_id`,`active_key_2a35c787dd96c557d1f04c69`);

CREATE INDEX `idx_source_24272aa06f8ae38879e405e9` ON `device_supply_commitments` (`deleted_at`);

ALTER TABLE `device_supply_commitments` ADD COLUMN `active_key_e2cf5fe6d73c0bdc0d27fd7a` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_e2cf5fe6d73c0bdc0d27fd7a` ON `device_supply_commitments` (`source_id`,`variant_id`,`active_key_e2cf5fe6d73c0bdc0d27fd7a`);

CREATE INDEX `idx_source_c4cf9027f05700df0c5976fd` ON `device_variant_tracking` (`deleted_at`);

ALTER TABLE `device_variant_tracking` ADD COLUMN `active_key_1199e512038860d7890f97f3` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_1199e512038860d7890f97f3` ON `device_variant_tracking` (`inventory_item_id`,`active_key_1199e512038860d7890f97f3`);

ALTER TABLE `device_variant_tracking` ADD COLUMN `active_key_f4148bdf63c7b8312af8efa9` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_f4148bdf63c7b8312af8efa9` ON `device_variant_tracking` (`variant_id`,`active_key_f4148bdf63c7b8312af8efa9`);

CREATE INDEX `idx_source_6756012390d64a32a448b003` ON `fulfillments` (`deleted_at`);

CREATE INDEX `idx_source_b5f9ec200932f70954872bb2` ON `fulfillments` (`location_id`);

CREATE INDEX `idx_source_7a9794bfd95daee6778e41b7` ON `fulfillments` (`shipping_option_id`);

CREATE INDEX `idx_source_f3e0988b3e6158c3802cc2f4` ON `fulfillment_addresses` (`deleted_at`);

CREATE INDEX `idx_source_1851017eeb0966bddbfa8bca` ON `fulfillment_items` (`deleted_at`);

CREATE INDEX `idx_source_d9fc401b6e96618395b58b76` ON `fulfillment_items` (`fulfillment_id`);

CREATE INDEX `idx_source_ee34b9aa8280ffcd0717b9ba` ON `fulfillment_items` (`inventory_item_id`);

CREATE INDEX `idx_source_86cc5d425824b45bce99433b` ON `fulfillment_items` (`line_item_id`);

CREATE INDEX `idx_source_51df855af91be622ec152b43` ON `fulfillment_labels` (`deleted_at`);

CREATE INDEX `idx_source_4d1a5b8bd191a728960899a4` ON `fulfillment_labels` (`fulfillment_id`);

CREATE INDEX `idx_source_b1ae793e383fd7a74bd3c754` ON `fulfillment_providers` (`deleted_at`);

CREATE INDEX `idx_source_9b6717e127c71559c2136868` ON `fulfillment_sets` (`deleted_at`);

CREATE INDEX `idx_source_4c38500ba2e7b735108979b2` ON `fulfillment_geo_zones` (`city`(128));

CREATE INDEX `idx_source_fe6041e287c12f7a3476a3e3` ON `fulfillment_geo_zones` (`country_code`(128));

CREATE INDEX `idx_source_820404db78243e9871993a21` ON `fulfillment_geo_zones` (`deleted_at`);

CREATE INDEX `idx_source_47f592bde5fc74e43d1a64e1` ON `fulfillment_geo_zones` (`province_code`(128));

CREATE INDEX `idx_source_cf6da5b2732f19b0cfa0e8d9` ON `fulfillment_geo_zones` (`service_zone_id`);

CREATE INDEX `idx_source_2f79595ddad9cdaffaea865b` ON `catalog_images` (`deleted_at`);

CREATE INDEX `idx_source_e25f8c01a191fa1868eb692f` ON `catalog_images` (`product_id`);

CREATE INDEX `idx_source_7dbdff864f562a575f8c2aeb` ON `catalog_images` (`rank`);

CREATE INDEX `idx_source_709c498700d8717e317617f6` ON `catalog_images` (`rank`,`product_id`);

CREATE INDEX `idx_source_80688e9495be62b72b91c07a` ON `catalog_images` (`url`(128));

CREATE INDEX `idx_source_0fda8d102114103ade7980cc` ON `catalog_images` (`url`(128),`rank`,`product_id`);

CREATE INDEX `idx_source_90baac2acc18aa3e2f64465a` ON `inventory_items` (`deleted_at`);

ALTER TABLE `inventory_items` ADD COLUMN `active_key_64bad780756ba441a34edf3c` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_64bad780756ba441a34edf3c` ON `inventory_items` (`sku`,`active_key_64bad780756ba441a34edf3c`);

CREATE INDEX `idx_source_decec4b6666d9efa414db8cd` ON `inventory_levels` (`deleted_at`);

CREATE INDEX `idx_source_b29251b9282c08ffde6f89b7` ON `inventory_levels` (`inventory_item_id`);

CREATE INDEX `idx_source_f66e66fd62c4e815c3b26fc0` ON `inventory_levels` (`location_id`);

ALTER TABLE `inventory_levels` ADD COLUMN `active_key_7a2a13286c517dcb0d8626a9` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_7a2a13286c517dcb0d8626a9` ON `inventory_levels` (`inventory_item_id`,`location_id`,`active_key_7a2a13286c517dcb0d8626a9`);

CREATE INDEX `idx_source_e5bd10a9f46db3af28eef16a` ON `fulfillment_location_providers` (`deleted_at`);

CREATE INDEX `idx_source_76e5897994bd6c48b9513e77` ON `fulfillment_location_providers` (`fulfillment_provider_id`);

CREATE INDEX `idx_source_763b60d4ec12957662e8a833` ON `fulfillment_location_providers` (`id`);

CREATE INDEX `idx_source_a3bf07573c3e494227e8b1b7` ON `fulfillment_location_providers` (`stock_location_id`);

CREATE INDEX `idx_source_8650b5a9a55ee8bd3a0f6c37` ON `fulfillment_location_sets` (`deleted_at`);

CREATE INDEX `idx_source_2e8845a9dd9ec2ddf25e1589` ON `fulfillment_location_sets` (`fulfillment_set_id`);

CREATE INDEX `idx_source_537c3f536cfee5fa4504eea2` ON `fulfillment_location_sets` (`id`);

CREATE INDEX `idx_source_64776c239c0964f93f85b47e` ON `fulfillment_location_sets` (`stock_location_id`);

CREATE INDEX `idx_source_a0e2f55b0f9c49abc2a431a5` ON `mail_accounts` (`deleted_at`);

CREATE INDEX `idx_source_ed2e5cbf4bc0f95b411d3c88` ON `mail_campaigns` (`deleted_at`);

CREATE INDEX `idx_source_279e7222be1a8ed5e54d754d` ON `mail_campaigns` (`status`);

CREATE INDEX `idx_source_f9d68f2ed0a594c51ca98bfe` ON `mail_contacts` (`deleted_at`);

ALTER TABLE `mail_deliveries` ADD COLUMN `active_key_ae6e6b5750bbfbe4932c7c4e` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ae6e6b5750bbfbe4932c7c4e` ON `mail_deliveries` (`campaign_id`,`contact_id`,`active_key_ae6e6b5750bbfbe4932c7c4e`);

CREATE INDEX `idx_source_95af7f3effebf45a5a046cc3` ON `mail_deliveries` (`campaign_id`,`status`(128));

CREATE INDEX `idx_source_84419f419dafa5b39832bc2d` ON `mail_deliveries` (`deleted_at`);

CREATE INDEX `idx_source_c2562e832d573debd9fa7c94` ON `mail_deliveries` (`provider_id`);

CREATE INDEX `idx_source_48d052f964dbaa0316e07fe7` ON `mail_events` (`deleted_at`);

ALTER TABLE `mail_events` ADD COLUMN `active_key_a06087372caafa5bee6bad48` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_a06087372caafa5bee6bad48` ON `mail_events` (`event_id`,`active_key_a06087372caafa5bee6bad48`);

CREATE INDEX `idx_source_af0760ed437beeb611843907` ON `mail_messages` (`account_id`,`folder`);

ALTER TABLE `mail_messages` ADD COLUMN `active_key_83fb94f85745e2f97aeda1c0` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_83fb94f85745e2f97aeda1c0` ON `mail_messages` (`account_id`,`provider_id`,`active_key_83fb94f85745e2f97aeda1c0`);

CREATE INDEX `idx_source_eae9e438d7e6d2d77e5c1e1c` ON `mail_messages` (`deleted_at`);

CREATE INDEX `idx_source_c6ee1472f30dd9981b0a3060` ON `mail_messages` (`folder`,`read`);

CREATE INDEX `idx_source_0a0529ef4fcd22f939a898a1` ON `mail_templates` (`deleted_at`);

CREATE INDEX `idx_source_6c4f2c2e197290ef859d71ad` ON `mollie_configs` (`deleted_at`);

CREATE INDEX `idx_source_7ec792fff1a2299f87efb4ff` ON `historical_notifications` (`deleted_at`);

CREATE INDEX `idx_source_7b12bb3434af92a30b7d730e` ON `historical_notifications` (`provider_id`);

CREATE INDEX `idx_source_315ca286758b958d955dd5cf` ON `historical_notifications` (`receiver_id`);

CREATE INDEX `idx_source_5e564279b91f5aafb084798b` ON `notification_providers` (`deleted_at`);

CREATE INDEX `idx_source_4b3b70978cecc008cb75854d` ON `orders` (`billing_address_id`);

CREATE INDEX `idx_source_28f36cf3495859bfb06ba3d7` ON `orders` (`currency_code`);

ALTER TABLE `orders` ADD COLUMN `active_key_7e690e4a573d7580a53b3b47` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_7e690e4a573d7580a53b3b47` ON `orders` (`custom_display_id`,`active_key_7e690e4a573d7580a53b3b47`);

CREATE INDEX `idx_source_3b07293ac2e02971e20403e4` ON `orders` (`customer_id`);

CREATE INDEX `idx_source_fe53abc99a007f9fa136cf33` ON `orders` (`deleted_at`);

CREATE INDEX `idx_source_f2e7642a47d96c5098efef59` ON `orders` (`display_id`);

CREATE INDEX `idx_source_c6a06021e62c5368dd026bb7` ON `orders` (`is_draft_order`);

CREATE INDEX `idx_source_f49ed93fbd74addbc0e6a0ff` ON `orders` (`region_id`);

CREATE INDEX `idx_source_44368dbfab2a42dabf8d07c1` ON `orders` (`sales_channel_id`);

CREATE INDEX `idx_source_20458946502264a94477f517` ON `orders` (`shipping_address_id`);

CREATE INDEX `idx_source_885e719ebbcdc38371618e2a` ON `order_addresses` (`customer_id`);

CREATE INDEX `idx_source_7db3419be6a4ca55cf35dfc3` ON `order_addresses` (`deleted_at`);

CREATE INDEX `idx_source_4ce612ded5ce3e17cdae6c32` ON `order_carts` (`cart_id`);

CREATE INDEX `idx_source_a75f3f14adead4e3e8b4dc16` ON `order_carts` (`deleted_at`);

CREATE INDEX `idx_source_19306507c16318ec34fbb7f8` ON `order_carts` (`id`);

CREATE INDEX `idx_source_a6c2f12ea2519b42a1ce6ac9` ON `order_carts` (`order_id`);

CREATE INDEX `idx_source_81ed6ce8eb2ad74c822d02a8` ON `order_changes` (`change_type`(128));

CREATE INDEX `idx_source_209ff5a7ce45617391d711ef` ON `order_changes` (`claim_id`);

CREATE INDEX `idx_source_21b4d6ec452dbc8fbbbf5ba1` ON `order_changes` (`deleted_at`);

CREATE INDEX `idx_source_8c2fecae5b71a7c00eaaf49e` ON `order_changes` (`exchange_id`);

CREATE INDEX `idx_source_5e49344085955fc2cbe39ca4` ON `order_changes` (`order_id`);

CREATE INDEX `idx_source_fdab94548bfb0a158fce0a62` ON `order_changes` (`order_id`,`version`);

CREATE INDEX `idx_source_9bdecb1054c7f84440969b62` ON `order_changes` (`return_id`);

CREATE INDEX `idx_source_6922ec84be0dc754bdcc41a3` ON `order_changes` (`status`);

CREATE INDEX `idx_source_617704637b21ccfacc5811c4` ON `order_changes` (`order_id`,`version`);

CREATE INDEX `idx_source_e9ee2b8b988b31d26ec1c0fb` ON `order_change_actions` (`claim_id`);

CREATE INDEX `idx_source_8390fb1009b7038173296186` ON `order_change_actions` (`deleted_at`);

CREATE INDEX `idx_source_9c2a85e307c115120ebdef31` ON `order_change_actions` (`exchange_id`);

CREATE INDEX `idx_source_c913e826616edae0cd3c2899` ON `order_change_actions` (`order_change_id`);

CREATE INDEX `idx_source_11fc737da18cb246244029a8` ON `order_change_actions` (`order_id`);

CREATE INDEX `idx_source_ae065333f73d4a4b952b8edb` ON `order_change_actions` (`ordering`);

CREATE INDEX `idx_source_32068d12deeb9cdfb92b8e10` ON `order_change_actions` (`return_id`);

CREATE INDEX `idx_source_b936db4de10bf5e9ecc7ecd0` ON `order_claims` (`deleted_at`);

CREATE INDEX `idx_source_4bc6bc2dc3f2d56bb3957163` ON `order_claims` (`display_id`);

CREATE INDEX `idx_source_d2da32a710adda21dff2d920` ON `order_claims` (`order_id`);

CREATE INDEX `idx_source_a10a8338919a9ede9047bd01` ON `order_claims` (`return_id`);

CREATE INDEX `idx_source_e859302a24b5c2f50109b635` ON `order_claim_items` (`claim_id`);

CREATE INDEX `idx_source_f6a5a1fa2f5675e841f13a42` ON `order_claim_items` (`deleted_at`);

CREATE INDEX `idx_source_2d6c1e955107b76860fd31e3` ON `order_claim_items` (`item_id`);

CREATE INDEX `idx_source_f923890cc576e91aa29c7a83` ON `order_claim_item_images` (`claim_item_id`);

CREATE INDEX `idx_source_fdc6541175c239860f437f3d` ON `order_claim_item_images` (`deleted_at`);

CREATE INDEX `idx_source_089a377ef28ce85b70b5e5f1` ON `order_credit_lines` (`deleted_at`);

CREATE INDEX `idx_source_a2716537254ec8d713c75c8c` ON `order_credit_lines` (`order_id`);

CREATE INDEX `idx_source_6f3380b4fe166615c86c49ab` ON `order_credit_lines` (`order_id`,`version`);

CREATE INDEX `idx_source_5580491d5fa59464cd3699ef` ON `order_exchanges` (`deleted_at`);

CREATE INDEX `idx_source_97634f6672872a67c41a5cbc` ON `order_exchanges` (`display_id`);

CREATE INDEX `idx_source_3e097433610a739b4f1afad0` ON `order_exchanges` (`order_id`);

CREATE INDEX `idx_source_bb9743890fdd715e6a125c8e` ON `order_exchanges` (`return_id`);

CREATE INDEX `idx_source_649415d4edf010de1e744028` ON `order_exchange_items` (`deleted_at`);

CREATE INDEX `idx_source_4652b189bfff42ec9b391d85` ON `order_exchange_items` (`exchange_id`);

CREATE INDEX `idx_source_74dd784a1567f82d5d3a5e4e` ON `order_exchange_items` (`item_id`);

CREATE INDEX `idx_source_e7094b1f67ff836e04878a7e` ON `order_fulfillments` (`deleted_at`);

CREATE INDEX `idx_source_e532ef33e7364379e791e9ba` ON `order_fulfillments` (`fulfillment_id`);

CREATE INDEX `idx_source_33dee520443c73cba25c9ebb` ON `order_fulfillments` (`id`);

CREATE INDEX `idx_source_213c36c0127498c5c0010982` ON `order_fulfillments` (`order_id`);

CREATE INDEX `idx_source_8b195e1e617e7541439b4d73` ON `order_items` (`deleted_at`);

CREATE INDEX `idx_source_94d791cb986be412fcbc52fb` ON `order_items` (`item_id`);

CREATE INDEX `idx_source_bb5f9b9d98296292b1a5acd0` ON `order_items` (`order_id`);

CREATE INDEX `idx_source_dcf3f4d73e9efd5bbb9ed892` ON `order_items` (`order_id`,`version`);

CREATE INDEX `idx_source_90c6c41801700bba0dad833a` ON `order_line_items` (`product_type_id`);

CREATE INDEX `idx_source_f2b3c07fb2ce0b290353fcf9` ON `order_line_items` (`product_id`);

CREATE INDEX `idx_source_fff9df4af2b01edc1acc4b66` ON `order_line_items` (`variant_id`);

CREATE INDEX `idx_source_08554fbfe2809a43b476dc16` ON `order_line_adjustments` (`item_id`);

CREATE INDEX `idx_source_278802566c131523725ad39c` ON `order_line_tax_lines` (`item_id`);

CREATE INDEX `idx_source_86334025c09219f8fe4d0b69` ON `order_payment_collections` (`deleted_at`);

CREATE INDEX `idx_source_2562a349f4f7f4cd781c514d` ON `order_payment_collections` (`id`);

CREATE INDEX `idx_source_42959d37033eb33403f5a83e` ON `order_payment_collections` (`order_id`);

CREATE INDEX `idx_source_93ac3f2c33af75c7da1ac8e9` ON `order_payment_collections` (`payment_collection_id`);

CREATE INDEX `idx_source_71e41aa27ba36a79f813a4be` ON `order_promotions` (`deleted_at`);

CREATE INDEX `idx_source_64ba9ef88e0f5d4217973d1f` ON `order_promotions` (`id`);

CREATE INDEX `idx_source_18248320e40e1bd80953713b` ON `order_promotions` (`order_id`);

CREATE INDEX `idx_source_4c5a778a8383ec67d4c870f7` ON `order_promotions` (`promotion_id`);

CREATE INDEX `idx_source_91e062c819fef96503d01c85` ON `order_shipping_versions` (`claim_id`);

CREATE INDEX `idx_source_3a40ba41d70a443eb41b99ee` ON `order_shipping_versions` (`deleted_at`);

CREATE INDEX `idx_source_8807b09de9ead7e749827cde` ON `order_shipping_versions` (`exchange_id`);

CREATE INDEX `idx_source_05e699ad2e690528a9d0fef8` ON `order_shipping_versions` (`shipping_method_id`);

CREATE INDEX `idx_source_d351c62423baaba8ef2dee4f` ON `order_shipping_versions` (`order_id`);

CREATE INDEX `idx_source_977b7399774182c79990b137` ON `order_shipping_versions` (`order_id`,`version`);

CREATE INDEX `idx_source_ee68dfd229956ae14ac2fa66` ON `order_shipping_versions` (`return_id`);

CREATE INDEX `idx_source_662ee35bbbae28fdbe6abec7` ON `order_shipping_versions` (`shipping_method_id`);

CREATE INDEX `idx_source_89bf19b3d5be38a3fabb6846` ON `order_shipping_methods` (`shipping_option_id`);

CREATE INDEX `idx_source_e3a68c70310fab54d71bbeb8` ON `order_shipping_adjustments` (`shipping_method_id`);

ALTER TABLE `order_shipping_adjustments` ADD COLUMN `active_key_ea58f9d7444a63c8765037c7` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ea58f9d7444a63c8765037c7` ON `order_shipping_adjustments` (`version`,`shipping_method_id`,`active_key_ea58f9d7444a63c8765037c7`);

CREATE INDEX `idx_source_ea8e6fb0ed8bd736952ede12` ON `order_shipping_tax_lines` (`shipping_method_id`);

CREATE INDEX `idx_source_5afa06bf1680a01a1ae3bf5e` ON `order_summaries` (`deleted_at`);

CREATE INDEX `idx_source_9612efb7adab55453e0b8d2b` ON `order_summaries` (`order_id`,`version`);

CREATE INDEX `idx_source_ab4a75ea890c617753c2d7a2` ON `order_transactions` (`claim_id`);

CREATE INDEX `idx_source_404eced94a07409c9acef226` ON `order_transactions` (`currency_code`);

CREATE INDEX `idx_source_9f01b7471fa241e359d53ce0` ON `order_transactions` (`exchange_id`);

CREATE INDEX `idx_source_cf53d0985d193a0dc1acff44` ON `order_transactions` (`order_id`);

CREATE INDEX `idx_source_9c14b44add3ce7be09534902` ON `order_transactions` (`order_id`,`version`);

CREATE INDEX `idx_source_1b71b0a2b53cea79d88a8d2b` ON `order_transactions` (`reference_id`);

CREATE INDEX `idx_source_6d95b40971b673f40fdd0e20` ON `order_transactions` (`return_id`);

CREATE INDEX `idx_source_b711b1af669676f5db0c04c5` ON `payments` (`deleted_at`);

CREATE INDEX `idx_source_bc086049f3f2d3050171d353` ON `payments` (`payment_collection_id`);

CREATE INDEX `idx_source_b787c6d7b956f8db4578fd64` ON `payments` (`payment_session_id`);

ALTER TABLE `payments` ADD COLUMN `active_key_ab1b6a4ef5fc2e255bd0cdb4` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ab1b6a4ef5fc2e255bd0cdb4` ON `payments` (`payment_session_id`,`active_key_ab1b6a4ef5fc2e255bd0cdb4`);

CREATE INDEX `idx_source_c7bd078f9d675258c1a376c6` ON `payments` (`provider_id`);

CREATE INDEX `idx_source_a14fe382d304c17e31c2d0fb` ON `payment_collections` (`deleted_at`);

CREATE INDEX `idx_source_72bd56ea6b19f1483ae5c109` ON `payment_providers` (`deleted_at`);

CREATE INDEX `idx_source_b9248659997fb45196ae7980` ON `payment_sessions` (`deleted_at`);

CREATE INDEX `idx_source_42b8abed96c928be0b8c439c` ON `payment_sessions` (`payment_collection_id`);

CREATE INDEX `idx_source_50b9b3ad657e8566b91fa848` ON `paypal_connections` (`deleted_at`);

CREATE INDEX `idx_source_6be02d76fa76e7f4cf1343c0` ON `paypal_connections` (`status`(128),`created_at` DESC);

CREATE INDEX `idx_source_629f30612ebda45d9a34bb35` ON `paypal_legacy_metrics` (`deleted_at`);

CREATE INDEX `idx_source_4e1e4f5c6d8019bd299a5d3d` ON `paypal_settings` (`deleted_at`);

CREATE INDEX `idx_source_2bc8bd63f420cbc2e5ad6042` ON `paypal_webhook_events` (`deleted_at`);

CREATE INDEX `idx_source_3f7f6ab9d0ad853e1cfe5459` ON `paypal_webhook_events` (`status`(128),`next_retry_at`);

CREATE INDEX `idx_source_39a55e48ec38a5627d35a96b` ON `paypal_webhook_events` (`transmission_id`);

CREATE INDEX `idx_source_710eb0b533b2d8c954531278` ON `paypal_webhook_events` (`event_type`(128));

CREATE UNIQUE INDEX `idx_source_c678845d185072689027e5d4` ON `paypal_webhook_events` (`event_id`);

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

CREATE INDEX `idx_source_11ee50a12924c33ec5b4eef8` ON `product_reviews` (`deleted_at`);

ALTER TABLE `product_reviews` ADD COLUMN `active_key_0051152aec00a03646fa6536` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_0051152aec00a03646fa6536` ON `product_reviews` (`product_id`,`customer_id`,`active_key_0051152aec00a03646fa6536`);

CREATE INDEX `idx_source_733fc2bc65c8b55f0e3973a8` ON `product_reviews` (`product_id`,`status`);

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

CREATE INDEX `idx_source_74e94709f5aef4856f89471f` ON `promotions` (`campaign_id`);

CREATE INDEX `idx_source_5da613a7788b030dbcda3ad1` ON `promotions` (`deleted_at`);

CREATE INDEX `idx_source_dbaf6c1a7f1c57ffa9f57098` ON `promotions` (`is_automatic`);

CREATE INDEX `idx_source_03ccaa41f0002cdab5f5c3ef` ON `promotions` (`status`);

CREATE INDEX `idx_source_c3026adad0deafa6b2e927a6` ON `promotions` (`type`);

ALTER TABLE `promotions` ADD COLUMN `active_key_0bc19fab3bb61a71a0331b56` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_0bc19fab3bb61a71a0331b56` ON `promotions` (`code`,`active_key_0bc19fab3bb61a71a0331b56`);

CREATE INDEX `idx_source_dde02f54c962ecbf66ee71d4` ON `promotion_application_methods` (`allocation`);

CREATE INDEX `idx_source_f0fd140d63e41313e792a3b6` ON `promotion_application_methods` (`target_type`);

CREATE INDEX `idx_source_8bce2e1ad57848c685a2ee62` ON `promotion_application_methods` (`type`);

CREATE INDEX `idx_source_b7d33e7369f37566600fb62a` ON `promotion_application_methods` (`currency_code`);

CREATE INDEX `idx_source_39767a9f6f0c5445235a3fcd` ON `promotion_application_methods` (`deleted_at`);

ALTER TABLE `promotion_application_methods` ADD COLUMN `active_key_462610d215f3536cd30e9074` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_462610d215f3536cd30e9074` ON `promotion_application_methods` (`promotion_id`,`active_key_462610d215f3536cd30e9074`);

CREATE INDEX `idx_source_19b09683012df8c4a17b7262` ON `promotion_campaigns` (`deleted_at`);

CREATE INDEX `idx_source_33bcae8db336bcf21581b93a` ON `promotion_campaign_budgets` (`type`);

ALTER TABLE `promotion_campaign_budgets` ADD COLUMN `active_key_2e1931f175c6ed5fe8571305` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_2e1931f175c6ed5fe8571305` ON `promotion_campaign_budgets` (`campaign_id`,`active_key_2e1931f175c6ed5fe8571305`);

CREATE INDEX `idx_source_695750a8a8ce46206f052356` ON `promotion_campaign_budgets` (`deleted_at`);

CREATE INDEX `idx_source_752ebae32e07d0df1aaf88d0` ON `promotion_budget_usages` (`budget_id`);

CREATE INDEX `idx_source_44c137fb7c1651f2483ac599` ON `promotion_budget_usages` (`deleted_at`);

CREATE INDEX `idx_source_83932c6857ae50df4058afc0` ON `promotion_rules` (`attribute`(128));

CREATE INDEX `idx_source_94635fbdec1a841b5a0dc12c` ON `promotion_rules` (`attribute`(128),`operator`);

CREATE INDEX `idx_source_929b1eb3e446067c0d202b2f` ON `promotion_rules` (`operator`,`attribute`(128),`id`);

CREATE INDEX `idx_source_c8c880fcede4d02b9a7f56be` ON `promotion_rules` (`deleted_at`);

CREATE INDEX `idx_source_a198d7fcb391288b3a9d28f3` ON `promotion_rules` (`operator`);

CREATE INDEX `idx_source_5cceb3170f3d68075aa0a097` ON `promotion_rule_values` (`deleted_at`);

CREATE INDEX `idx_source_26b596736405f66f1ecf1700` ON `promotion_rule_values` (`promotion_rule_id`);

CREATE INDEX `idx_source_229a59c51d0cb4ea0023eae1` ON `promotion_rule_values` (`promotion_rule_id`,`value`(128));

CREATE INDEX `idx_source_65020dd956ff81a1ca48a744` ON `promotion_rule_values` (`value`(128));

CREATE INDEX `idx_source_b27f0c601d1de8b6965ac1a7` ON `payment_refunds` (`deleted_at`);

CREATE INDEX `idx_source_0f620d75004790db3ca9ec8a` ON `payment_refunds` (`payment_id`);

CREATE INDEX `idx_source_499252ef124ea789379c85b5` ON `payment_refunds` (`refund_reason_id`);

CREATE INDEX `idx_source_fc74b4959361109633a9b78d` ON `payment_refund_reasons` (`deleted_at`);

CREATE INDEX `idx_source_69c907f08cafde8ea39357e7` ON `commerce_regions` (`deleted_at`);

CREATE INDEX `idx_source_910f7b721be25210ed3460f5` ON `commerce_countries` (`deleted_at`);

CREATE INDEX `idx_source_6e31dce258c02e3afb91a51e` ON `commerce_countries` (`region_id`);

CREATE UNIQUE INDEX `idx_source_87f6ec973e5899911b745351` ON `commerce_countries` (`region_id`,`iso_2`);

CREATE INDEX `idx_source_89adbaf0bd564b891f763e42` ON `commerce_region_payment_providers` (`deleted_at`);

CREATE INDEX `idx_source_b576bbd8709286c1b4260e90` ON `commerce_region_payment_providers` (`id`);

CREATE INDEX `idx_source_9423a567d8b5625d65bd8124` ON `commerce_region_payment_providers` (`payment_provider_id`);

CREATE INDEX `idx_source_f888006a65f5df8fe891cd18` ON `commerce_region_payment_providers` (`region_id`);

CREATE INDEX `idx_source_8661bc1788d7eeb6e3bea518` ON `inventory_reservations` (`deleted_at`);

CREATE INDEX `idx_source_69cce2620646ac4c90c7124b` ON `inventory_reservations` (`inventory_item_id`);

CREATE INDEX `idx_source_9464fdef706dccf46f71e671` ON `inventory_reservations` (`line_item_id`);

CREATE INDEX `idx_source_59a4b87863074548ac6b1543` ON `inventory_reservations` (`location_id`);

CREATE INDEX `idx_source_342f51001b1baf3eac3fe084` ON `returns` (`claim_id`);

CREATE INDEX `idx_source_bfa2fa499f5affaa5b6fe36f` ON `returns` (`display_id`);

CREATE INDEX `idx_source_0b8b3bffabe3457646ee2f8b` ON `returns` (`exchange_id`);

CREATE INDEX `idx_source_300b7f3d4f0516ed68ea60b7` ON `returns` (`order_id`);

CREATE INDEX `idx_source_6adc2807e4ef7030d709371a` ON `return_fulfillments` (`deleted_at`);

CREATE INDEX `idx_source_c1419bf3e2075ad653c401cf` ON `return_fulfillments` (`fulfillment_id`);

CREATE INDEX `idx_source_d8a26f861295e16692d99066` ON `return_fulfillments` (`id`);

CREATE INDEX `idx_source_65557b7bc9ca8ae016981749` ON `return_fulfillments` (`return_id`);

CREATE INDEX `idx_source_728adba2252bb444cc67dc6a` ON `return_items` (`deleted_at`);

CREATE INDEX `idx_source_902951ebbca54c7013882dad` ON `return_items` (`item_id`);

CREATE INDEX `idx_source_edd91060708a960bd10c5990` ON `return_items` (`reason_id`);

CREATE INDEX `idx_source_af664a3e9825b363dea1e7a9` ON `return_items` (`return_id`);

CREATE INDEX `idx_source_e783f2034d126e29703393dc` ON `return_reasons` (`parent_return_reason_id`);

CREATE INDEX `idx_source_926cd8936a9c8282634a2a2d` ON `return_reasons` (`value`(128));

CREATE INDEX `idx_source_09b96c6170a705e97717ef0c` ON `commerce_sales_channels` (`deleted_at`);

CREATE INDEX `idx_source_dd9214d1d15a03092f34b031` ON `commerce_channel_locations` (`deleted_at`);

CREATE INDEX `idx_source_0180848ce31babf95d45edcf` ON `commerce_channel_locations` (`id`);

CREATE INDEX `idx_source_5fc2c9522b02d08c7d79b12c` ON `commerce_channel_locations` (`sales_channel_id`);

CREATE INDEX `idx_source_666e89cf505ace7e1a145d25` ON `commerce_channel_locations` (`stock_location_id`);

CREATE INDEX `idx_source_e0307781c4b813551465e0fe` ON `seo_crawler_visits` (`deleted_at`);

CREATE INDEX `idx_source_e9ad7343590e50e2deae150d` ON `seo_crawler_visits` (`occurred_at`,`crawler_name`(128));

CREATE INDEX `idx_source_575fc8b07d7e27207a95b95b` ON `seo_crawler_visits` (`source_id`);

CREATE INDEX `idx_source_734aa2be4bcbe8070dcff523` ON `seo_evidence` (`deleted_at`);

CREATE INDEX `idx_source_8812f2950cb1c9637a785097` ON `seo_evidence` (`run_id`);

CREATE INDEX `idx_source_29b318a4f6e2bc8a489555c8` ON `seo_monitors` (`deleted_at`);

CREATE INDEX `idx_source_47fcc2790d4c95c701b2c4f8` ON `seo_pages` (`deleted_at`);

CREATE INDEX `idx_source_08d457710155dc9c67d715b9` ON `seo_providers` (`deleted_at`);

ALTER TABLE `seo_providers` ADD COLUMN `active_key_ae04d64f6013518b4c6b694d` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ae04d64f6013518b4c6b694d` ON `seo_providers` (`provider`,`active_key_ae04d64f6013518b4c6b694d`);

CREATE INDEX `idx_source_5bd2b32d2c5b5a7e517638de` ON `seo_runs` (`deleted_at`);

CREATE INDEX `idx_source_4c75d45d5e58cc7fd8839155` ON `seo_runs` (`monitor_id`,`started_at`);

CREATE INDEX `idx_source_a488ecd5b37b7e9a7792c138` ON `seo_settings` (`deleted_at`);

CREATE INDEX `idx_source_aa65d3485a4967681f700d60` ON `seo_sources` (`deleted_at`);

CREATE INDEX `idx_source_86de7730be7acc1d1e5a2404` ON `seo_submissions` (`deleted_at`);

CREATE INDEX `idx_source_68dfdd49becd981c6af4377d` ON `seo_traffic_visits` (`deleted_at`);

CREATE INDEX `idx_source_15a7f483837aed76cf0520d6` ON `seo_traffic_visits` (`log_source_id`);

CREATE INDEX `idx_source_125394e77909298ef8187a70` ON `seo_traffic_visits` (`occurred_at`,`source`(128));

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

CREATE INDEX `idx_source_be0a7e0eb08b154bc718e5a7` ON `fulfillment_option_rules` (`deleted_at`);

CREATE INDEX `idx_source_3c1ace3e5b0a159d6dabffd0` ON `fulfillment_option_rules` (`shipping_option_id`);

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

CREATE INDEX `idx_source_90ac1a3a9c5c2e76ea4cf41e` ON `tax_providers` (`deleted_at`);

ALTER TABLE `tax_rates` ADD COLUMN `active_key_51b5544850cd23fdae1d46d5` TINYINT GENERATED ALWAYS AS (IF(`is_default` = 1 AND `deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_51b5544850cd23fdae1d46d5` ON `tax_rates` (`tax_region_id`,`active_key_51b5544850cd23fdae1d46d5`);

CREATE INDEX `idx_source_f89fbb975d3b318610173f27` ON `tax_rates` (`deleted_at`);

CREATE INDEX `idx_source_735e531d1f524439119d56ed` ON `tax_rates` (`tax_region_id`);

CREATE INDEX `idx_source_be4b95d01edeca3ebf5ac6f3` ON `tax_rate_rules` (`deleted_at`);

CREATE INDEX `idx_source_dc1f83af5987b708edcda54f` ON `tax_rate_rules` (`reference_id`);

CREATE INDEX `idx_source_d55dde23e4fb85e593318b8b` ON `tax_rate_rules` (`tax_rate_id`);

ALTER TABLE `tax_rate_rules` ADD COLUMN `active_key_d1071cb51d39ddcaae642afb` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_d1071cb51d39ddcaae642afb` ON `tax_rate_rules` (`tax_rate_id`,`reference_id`,`active_key_d1071cb51d39ddcaae642afb`);

CREATE INDEX `idx_source_c24632082c487992a1f5dde5` ON `tax_regions` (`deleted_at`);

CREATE INDEX `idx_source_92fadb6a27196758b2da3ed2` ON `tax_regions` (`parent_id`);

CREATE INDEX `idx_source_047a333acb077b372702c521` ON `tax_regions` (`provider_id`);

CREATE INDEX `idx_source_de1416bfe7db4ecf1fe57f18` ON `webhook_endpoints` (`deleted_at`);

ALTER TABLE `promotion_buy_rules` ADD CONSTRAINT `fk_source_91b67b4a892e981ffd3b34a2` FOREIGN KEY (`application_method_id`) REFERENCES `promotion_application_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_buy_rules` ADD CONSTRAINT `fk_source_1869c84d42d520bc6e069101` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_target_rules` ADD CONSTRAINT `fk_source_69269ec0caa922d90124218f` FOREIGN KEY (`application_method_id`) REFERENCES `promotion_application_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_target_rules` ADD CONSTRAINT `fk_source_11e4242cdf12b6d21709b9bf` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_captures` ADD CONSTRAINT `fk_source_d53cde0eeccbb90eb9fd6125` FOREIGN KEY (`payment_id`) REFERENCES `payments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `carts` ADD CONSTRAINT `fk_source_59745912d6e6848900ec5925` FOREIGN KEY (`billing_address_id`) REFERENCES `cart_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `carts` ADD CONSTRAINT `fk_source_138495b9ed5c763526e103f5` FOREIGN KEY (`shipping_address_id`) REFERENCES `cart_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `cart_line_items` ADD CONSTRAINT `fk_source_351fe6c32ff9a453bfd21b5b` FOREIGN KEY (`cart_id`) REFERENCES `carts` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_line_adjustments` ADD CONSTRAINT `fk_source_ba1b79afeb8daf43e1d955c1` FOREIGN KEY (`item_id`) REFERENCES `cart_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_line_tax_lines` ADD CONSTRAINT `fk_source_63732f4985f9c52032eef6e8` FOREIGN KEY (`item_id`) REFERENCES `cart_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_methods` ADD CONSTRAINT `fk_source_9c6644cd75f5ad1d713a746b` FOREIGN KEY (`cart_id`) REFERENCES `carts` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_adjustments` ADD CONSTRAINT `fk_source_b1b13c77283fad95f02c9d38` FOREIGN KEY (`shipping_method_id`) REFERENCES `cart_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_tax_lines` ADD CONSTRAINT `fk_source_786eca44e110a0b003f466e3` FOREIGN KEY (`shipping_method_id`) REFERENCES `cart_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_credit_lines` ADD CONSTRAINT `fk_source_552506ec559afc1a86345edf` FOREIGN KEY (`cart_id`) REFERENCES `carts` (`id`) ON UPDATE CASCADE;

ALTER TABLE `historical_customer_memberships` ADD CONSTRAINT `fk_source_95e0d71b3b37fc75ca5bc704` FOREIGN KEY (`customer_group_id`) REFERENCES `historical_customer_groups` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `historical_customer_memberships` ADD CONSTRAINT `fk_source_8e56e9252919a7e4a52ae4b2` FOREIGN KEY (`customer_id`) REFERENCES `historical_customer_subjects` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_97cb8eb1b6f6e6c9bb78b16e` FOREIGN KEY (`delivery_address_id`) REFERENCES `fulfillment_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_2937fa7fdc5363bc90975643` FOREIGN KEY (`provider_id`) REFERENCES `fulfillment_providers` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_3a1865a8b7043fd6835819db` FOREIGN KEY (`shipping_option_id`) REFERENCES `fulfillment_shipping_options` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_items` ADD CONSTRAINT `fk_source_f893b5c3b152ed4220b97571` FOREIGN KEY (`fulfillment_id`) REFERENCES `fulfillments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_labels` ADD CONSTRAINT `fk_source_fcbe035f719a8f4d40843d57` FOREIGN KEY (`fulfillment_id`) REFERENCES `fulfillments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_geo_zones` ADD CONSTRAINT `fk_source_7039181dda45c2ce8a586b25` FOREIGN KEY (`service_zone_id`) REFERENCES `fulfillment_service_zones` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `catalog_images` ADD CONSTRAINT `fk_source_ede5b1bdc2ef9a86887ae7a8` FOREIGN KEY (`product_id`) REFERENCES `catalog_products` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `inventory_levels` ADD CONSTRAINT `fk_source_39c98920e8353c5f4bf645f6` FOREIGN KEY (`inventory_item_id`) REFERENCES `inventory_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `historical_notifications` ADD CONSTRAINT `fk_source_d3f44bc1a440a1e839dd049e` FOREIGN KEY (`provider_id`) REFERENCES `notification_providers` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `orders` ADD CONSTRAINT `fk_source_501d674088335aea1bf5a074` FOREIGN KEY (`billing_address_id`) REFERENCES `order_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `orders` ADD CONSTRAINT `fk_source_f8d244ce50dc943c115111eb` FOREIGN KEY (`shipping_address_id`) REFERENCES `order_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `order_changes` ADD CONSTRAINT `fk_source_abb9be26e703b0f956772e4d` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_change_actions` ADD CONSTRAINT `fk_source_f898544777df1d0736a2ff86` FOREIGN KEY (`order_change_id`) REFERENCES `order_changes` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_credit_lines` ADD CONSTRAINT `fk_source_8fee8374693e78d98a751f75` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_items` ADD CONSTRAINT `fk_source_0dc0380d957f9b0133986098` FOREIGN KEY (`item_id`) REFERENCES `order_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_items` ADD CONSTRAINT `fk_source_695d37a2fed81d25b245a0a8` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_line_items` ADD CONSTRAINT `fk_source_fbac8dcb2d22873b0c7ec13f` FOREIGN KEY (`totals_id`) REFERENCES `order_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_line_adjustments` ADD CONSTRAINT `fk_source_52c07ea69a779d5d438f77b8` FOREIGN KEY (`item_id`) REFERENCES `order_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_line_tax_lines` ADD CONSTRAINT `fk_source_81c1c4ff31dd2dd7b066b3a9` FOREIGN KEY (`item_id`) REFERENCES `order_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_shipping_versions` ADD CONSTRAINT `fk_source_25b8ffc6ea80df741bcc2b5e` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_shipping_adjustments` ADD CONSTRAINT `fk_source_5ba83a53242914f6ef686a58` FOREIGN KEY (`shipping_method_id`) REFERENCES `order_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_shipping_tax_lines` ADD CONSTRAINT `fk_source_84816d64a30942f1417f3e7f` FOREIGN KEY (`shipping_method_id`) REFERENCES `order_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_summaries` ADD CONSTRAINT `fk_source_8dc648f92caca8e96a47b60f` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `order_transactions` ADD CONSTRAINT `fk_source_cede515b8bcfdf0dcb6b2ecb` FOREIGN KEY (`order_id`) REFERENCES `orders` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payments` ADD CONSTRAINT `fk_source_5ecd7def6ee14d7ec86997a3` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_collection_providers` ADD CONSTRAINT `fk_source_1b669e4c5a658caa85ac4151` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_collection_providers` ADD CONSTRAINT `fk_source_4e01fa660b250025e928ce6a` FOREIGN KEY (`payment_provider_id`) REFERENCES `payment_providers` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_sessions` ADD CONSTRAINT `fk_source_cd417994fcbf48dc086631c9` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

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

ALTER TABLE `promotions` ADD CONSTRAINT `fk_source_762fe8c64566662cbe806085` FOREIGN KEY (`campaign_id`) REFERENCES `promotion_campaigns` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `promotion_application_methods` ADD CONSTRAINT `fk_source_c83cafdaa4cf9c5c4d38f129` FOREIGN KEY (`promotion_id`) REFERENCES `promotions` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_campaign_budgets` ADD CONSTRAINT `fk_source_a9a7333b1e954fc49be15c08` FOREIGN KEY (`campaign_id`) REFERENCES `promotion_campaigns` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_budget_usages` ADD CONSTRAINT `fk_source_95daed9f0c2ef11d61cc4558` FOREIGN KEY (`budget_id`) REFERENCES `promotion_campaign_budgets` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_order_rules` ADD CONSTRAINT `fk_source_30e6c1a172b48201f6e74fdc` FOREIGN KEY (`promotion_id`) REFERENCES `promotions` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_order_rules` ADD CONSTRAINT `fk_source_e8b50ee61446cf5a69809b2f` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_rule_values` ADD CONSTRAINT `fk_source_5f358760aaa8da8dd9b0cbe6` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_refunds` ADD CONSTRAINT `fk_source_1873177dfc20cf7bd44b1606` FOREIGN KEY (`payment_id`) REFERENCES `payments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `commerce_countries` ADD CONSTRAINT `fk_source_c62c42b3b2ba01d0a4e12093` FOREIGN KEY (`region_id`) REFERENCES `commerce_regions` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `inventory_reservations` ADD CONSTRAINT `fk_source_f67ed0571b02889b7cdadb66` FOREIGN KEY (`inventory_item_id`) REFERENCES `inventory_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `return_reasons` ADD CONSTRAINT `fk_source_cf0c1759be48f3f9e295074a` FOREIGN KEY (`parent_return_reason_id`) REFERENCES `return_reasons` (`id`);

ALTER TABLE `fulfillment_service_zones` ADD CONSTRAINT `fk_source_41ee77d2322d75a7dc4fb7bb` FOREIGN KEY (`fulfillment_set_id`) REFERENCES `fulfillment_sets` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_b5ee90c53fbc6c39f5311fc7` FOREIGN KEY (`provider_id`) REFERENCES `fulfillment_providers` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_e078d8dff4a0b585a9295cf2` FOREIGN KEY (`service_zone_id`) REFERENCES `fulfillment_service_zones` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_8b9cb28f9981c97bbec4ffc9` FOREIGN KEY (`shipping_option_type_id`) REFERENCES `fulfillment_option_types` (`id`) ON UPDATE CASCADE;

ALTER TABLE `fulfillment_shipping_options` ADD CONSTRAINT `fk_source_d2edc156282c291dec6ddf56` FOREIGN KEY (`shipping_profile_id`) REFERENCES `fulfillment_shipping_profiles` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_option_rules` ADD CONSTRAINT `fk_source_17b182ec282eb7b6152c110a` FOREIGN KEY (`shipping_option_id`) REFERENCES `fulfillment_shipping_options` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `inventory_locations` ADD CONSTRAINT `fk_source_a6e6f417f5b505788abeb94a` FOREIGN KEY (`address_id`) REFERENCES `inventory_location_addresses` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `commerce_store_currencies` ADD CONSTRAINT `fk_source_8bc7cc47e67aea2cb767b374` FOREIGN KEY (`store_id`) REFERENCES `commerce_stores` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `commerce_store_locales` ADD CONSTRAINT `fk_source_37766b489ef7d0ad92df27c1` FOREIGN KEY (`store_id`) REFERENCES `commerce_stores` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `tax_rates` ADD CONSTRAINT `fk_source_e8d5f07735e10af8070732e0` FOREIGN KEY (`tax_region_id`) REFERENCES `tax_regions` (`id`) ON DELETE CASCADE;

ALTER TABLE `tax_rate_rules` ADD CONSTRAINT `fk_source_36bde9d484f953820e396ef4` FOREIGN KEY (`tax_rate_id`) REFERENCES `tax_rates` (`id`) ON DELETE CASCADE;
