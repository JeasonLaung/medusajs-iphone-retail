-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE cart_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO cart_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('cart_addresses','cart_line_adjustments','cart_line_items','cart_line_tax_lines','cart_payment_collections','cart_promotions','cart_runtime_schema','cart_shipping_adjustments','cart_shipping_methods','cart_shipping_tax_lines','carts'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

ALTER TABLE `carts` ADD CONSTRAINT `fk_source_59745912d6e6848900ec5925` FOREIGN KEY (`billing_address_id`) REFERENCES `cart_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `carts` ADD CONSTRAINT `fk_source_138495b9ed5c763526e103f5` FOREIGN KEY (`shipping_address_id`) REFERENCES `cart_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `cart_line_items` ADD CONSTRAINT `fk_source_351fe6c32ff9a453bfd21b5b` FOREIGN KEY (`cart_id`) REFERENCES `carts` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_line_adjustments` ADD CONSTRAINT `fk_source_ba1b79afeb8daf43e1d955c1` FOREIGN KEY (`item_id`) REFERENCES `cart_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_line_tax_lines` ADD CONSTRAINT `fk_source_63732f4985f9c52032eef6e8` FOREIGN KEY (`item_id`) REFERENCES `cart_line_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_methods` ADD CONSTRAINT `fk_source_9c6644cd75f5ad1d713a746b` FOREIGN KEY (`cart_id`) REFERENCES `carts` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_adjustments` ADD CONSTRAINT `fk_source_b1b13c77283fad95f02c9d38` FOREIGN KEY (`shipping_method_id`) REFERENCES `cart_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `cart_shipping_tax_lines` ADD CONSTRAINT `fk_source_786eca44e110a0b003f466e3` FOREIGN KEY (`shipping_method_id`) REFERENCES `cart_shipping_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Historical carts never restore possession, customer ownership or payment authority.
CREATE TABLE cart_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO cart_runtime_schema VALUES (1,1,'3556094145ffea41094586104e679857c9c3d7167237795fa36bfbbf2b4d54d0',TRUE,FALSE);
