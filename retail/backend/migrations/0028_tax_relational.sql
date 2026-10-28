-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE tax_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO tax_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('tax_providers','tax_rate_rules','tax_rates','tax_regions','tax_runtime_schema'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

ALTER TABLE `tax_rates` ADD CONSTRAINT `fk_source_e8d5f07735e10af8070732e0` FOREIGN KEY (`tax_region_id`) REFERENCES `tax_regions` (`id`) ON DELETE CASCADE;

ALTER TABLE `tax_rate_rules` ADD CONSTRAINT `fk_source_36bde9d484f953820e396ef4` FOREIGN KEY (`tax_rate_id`) REFERENCES `tax_rates` (`id`) ON DELETE CASCADE;

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Tax configuration readiness does not authorize cart writes or provider execution.
CREATE TABLE tax_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO tax_runtime_schema VALUES (1,1,'0a0c4ecd7dc58dcff6d60e8a1d35a7a1bf74c759664ab2c7b07fc9e455a241bf',TRUE,FALSE);
