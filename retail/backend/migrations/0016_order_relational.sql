-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE order_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO order_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('historical_customer_subjects','order_addresses','order_carts','order_change_actions','order_changes','order_claim_item_images','order_claim_items','order_claims','order_credit_lines','order_exchange_items','order_exchanges','order_fulfillments','order_items','order_line_adjustments','order_line_items','order_line_tax_lines','order_payment_collections','order_promotions','order_runtime_schema','order_shipping_adjustments','order_shipping_methods','order_shipping_tax_lines','order_shipping_versions','order_summaries','order_transactions','orders','return_fulfillments','return_items','return_reasons','returns'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

CREATE INDEX `idx_source_57fd489466a8ccdb6bb6df87` ON `historical_customer_subjects` (`deleted_at`);

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

ALTER TABLE `return_reasons` ADD CONSTRAINT `fk_source_cf0c1759be48f3f9e295074a` FOREIGN KEY (`parent_return_reason_id`) REFERENCES `return_reasons` (`id`);

ALTER TABLE `orders` ADD CONSTRAINT `ck_order_enum_97776694abcfdf449151a0af` CHECK (`status` IN ('pending','completed','draft','archived','canceled','requires_action'));

ALTER TABLE `order_changes` ADD CONSTRAINT `ck_order_enum_45193c0507ade4048d1be0e6` CHECK (`status` IN ('confirmed','declined','requested','pending','canceled'));

ALTER TABLE `order_claims` ADD CONSTRAINT `ck_order_enum_2960287221bd05d89ffbc226` CHECK (`type` IN ('refund','replace'));

ALTER TABLE `order_claim_items` ADD CONSTRAINT `ck_order_enum_e2a082ca9328e69a073e2947` CHECK (`reason` IN ('missing_item','wrong_item','production_failure','other'));

ALTER TABLE `returns` ADD CONSTRAINT `ck_order_enum_bfae0db85fb80add1eaba587` CHECK (`status` IN ('open','requested','received','partially_received','canceled'));

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Historical subjects have no login credentials or new-account grants.
CREATE TABLE order_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO order_runtime_schema VALUES (1,1,'3d27cb6968888bfaef5c87426fed1a9c7a1b8ac0005e9e614cec8d9d6ff74272',TRUE,FALSE);
