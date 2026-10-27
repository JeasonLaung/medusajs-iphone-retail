-- Inventory quantities and reservation evidence; runtime writes remain disabled.
SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

CREATE INDEX `idx_source_decec4b6666d9efa414db8cd` ON `inventory_levels` (`deleted_at`);

CREATE INDEX `idx_source_b29251b9282c08ffde6f89b7` ON `inventory_levels` (`inventory_item_id`);

CREATE INDEX `idx_source_f66e66fd62c4e815c3b26fc0` ON `inventory_levels` (`location_id`);

ALTER TABLE `inventory_levels` ADD COLUMN `active_key_7a2a13286c517dcb0d8626a9` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_7a2a13286c517dcb0d8626a9` ON `inventory_levels` (`inventory_item_id`,`location_id`,`active_key_7a2a13286c517dcb0d8626a9`);

CREATE INDEX `idx_source_8661bc1788d7eeb6e3bea518` ON `inventory_reservations` (`deleted_at`);

CREATE INDEX `idx_source_69cce2620646ac4c90c7124b` ON `inventory_reservations` (`inventory_item_id`);

CREATE INDEX `idx_source_9464fdef706dccf46f71e671` ON `inventory_reservations` (`line_item_id`);

CREATE INDEX `idx_source_59a4b87863074548ac6b1543` ON `inventory_reservations` (`location_id`);

ALTER TABLE `inventory_levels` ADD CONSTRAINT `fk_source_39c98920e8353c5f4bf645f6` FOREIGN KEY (`inventory_item_id`) REFERENCES `inventory_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `inventory_reservations` ADD CONSTRAINT `fk_source_f67ed0571b02889b7cdadb66` FOREIGN KEY (`inventory_item_id`) REFERENCES `inventory_items` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

CREATE TABLE inventory_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO inventory_runtime_schema VALUES (1,1,'14ef63b2931c9fb1dfa69c930d052c0707273c52729a3ae69186406d63dbd493',TRUE,FALSE);
