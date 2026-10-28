-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE geo_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO geo_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('fulfillment_geo_zones','geo_runtime_schema'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

CREATE INDEX `idx_source_4c38500ba2e7b735108979b2` ON `fulfillment_geo_zones` (`city`(128));

CREATE INDEX `idx_source_fe6041e287c12f7a3476a3e3` ON `fulfillment_geo_zones` (`country_code`(128));

CREATE INDEX `idx_source_820404db78243e9871993a21` ON `fulfillment_geo_zones` (`deleted_at`);

CREATE INDEX `idx_source_47f592bde5fc74e43d1a64e1` ON `fulfillment_geo_zones` (`province_code`(128));

CREATE INDEX `idx_source_cf6da5b2732f19b0cfa0e8d9` ON `fulfillment_geo_zones` (`service_zone_id`);

ALTER TABLE `fulfillment_geo_zones` ADD CONSTRAINT `fk_source_7039181dda45c2ce8a586b25` FOREIGN KEY (`service_zone_id`) REFERENCES `fulfillment_service_zones` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `fulfillment_geo_zones` ADD CONSTRAINT `ck_geo_enum_f732510fa1d72c6b1f5c0b31` CHECK (`type` IN ('country','province','city','zip'));

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Geo configuration readiness does not authorize cart writes or provider execution.
CREATE TABLE geo_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO geo_runtime_schema VALUES (1,1,'8675db551f62aeb156895c2cc0386857f48a9c153d09877d64ce456b7ccd38d9',TRUE,FALSE);
