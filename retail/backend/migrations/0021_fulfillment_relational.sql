-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE fulfillment_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO fulfillment_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('fulfillment_addresses','fulfillment_items','fulfillment_labels','fulfillment_location_providers','fulfillment_location_sets','fulfillment_option_rules','fulfillment_runtime_schema','fulfillments'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

CREATE INDEX `idx_source_e5bd10a9f46db3af28eef16a` ON `fulfillment_location_providers` (`deleted_at`);

CREATE INDEX `idx_source_76e5897994bd6c48b9513e77` ON `fulfillment_location_providers` (`fulfillment_provider_id`);

CREATE INDEX `idx_source_763b60d4ec12957662e8a833` ON `fulfillment_location_providers` (`id`);

CREATE INDEX `idx_source_a3bf07573c3e494227e8b1b7` ON `fulfillment_location_providers` (`stock_location_id`);

CREATE INDEX `idx_source_8650b5a9a55ee8bd3a0f6c37` ON `fulfillment_location_sets` (`deleted_at`);

CREATE INDEX `idx_source_2e8845a9dd9ec2ddf25e1589` ON `fulfillment_location_sets` (`fulfillment_set_id`);

CREATE INDEX `idx_source_537c3f536cfee5fa4504eea2` ON `fulfillment_location_sets` (`id`);

CREATE INDEX `idx_source_64776c239c0964f93f85b47e` ON `fulfillment_location_sets` (`stock_location_id`);

CREATE INDEX `idx_source_be0a7e0eb08b154bc718e5a7` ON `fulfillment_option_rules` (`deleted_at`);

CREATE INDEX `idx_source_3c1ace3e5b0a159d6dabffd0` ON `fulfillment_option_rules` (`shipping_option_id`);

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_97cb8eb1b6f6e6c9bb78b16e` FOREIGN KEY (`delivery_address_id`) REFERENCES `fulfillment_addresses` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_2937fa7fdc5363bc90975643` FOREIGN KEY (`provider_id`) REFERENCES `fulfillment_providers` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillments` ADD CONSTRAINT `fk_source_3a1865a8b7043fd6835819db` FOREIGN KEY (`shipping_option_id`) REFERENCES `fulfillment_shipping_options` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `fulfillment_items` ADD CONSTRAINT `fk_source_f893b5c3b152ed4220b97571` FOREIGN KEY (`fulfillment_id`) REFERENCES `fulfillments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_labels` ADD CONSTRAINT `fk_source_fcbe035f719a8f4d40843d57` FOREIGN KEY (`fulfillment_id`) REFERENCES `fulfillments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_option_rules` ADD CONSTRAINT `fk_source_17b182ec282eb7b6152c110a` FOREIGN KEY (`shipping_option_id`) REFERENCES `fulfillment_shipping_options` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_option_rules` ADD CONSTRAINT `ck_fulfillment_enum_1c42941d47daa944d46156dd` CHECK (`operator` IN ('in','eq','ne','gt','gte','lt','lte','nin'));

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Historical fulfillment evidence does not grant shipping, cancellation or inventory authority.
CREATE TABLE fulfillment_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO fulfillment_runtime_schema VALUES (1,1,'6736df15d94a4ed483777d5a757fff05cdf3c7ca44d619c9da2398841b192de6',TRUE,FALSE);
