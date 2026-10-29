-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE promotion_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO promotion_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('promotion_application_methods','promotion_budget_usages','promotion_buy_rules','promotion_campaign_budgets','promotion_campaigns','promotion_order_rules','promotion_rule_values','promotion_rules','promotion_runtime_schema','promotion_target_rules','promotions'));

SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';

SET SESSION time_zone='+00:00';

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

ALTER TABLE `promotion_buy_rules` ADD CONSTRAINT `fk_source_91b67b4a892e981ffd3b34a2` FOREIGN KEY (`application_method_id`) REFERENCES `promotion_application_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_buy_rules` ADD CONSTRAINT `fk_source_1869c84d42d520bc6e069101` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_target_rules` ADD CONSTRAINT `fk_source_69269ec0caa922d90124218f` FOREIGN KEY (`application_method_id`) REFERENCES `promotion_application_methods` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_target_rules` ADD CONSTRAINT `fk_source_11e4242cdf12b6d21709b9bf` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotions` ADD CONSTRAINT `fk_source_762fe8c64566662cbe806085` FOREIGN KEY (`campaign_id`) REFERENCES `promotion_campaigns` (`id`) ON UPDATE CASCADE ON DELETE SET NULL;

ALTER TABLE `promotion_application_methods` ADD CONSTRAINT `fk_source_c83cafdaa4cf9c5c4d38f129` FOREIGN KEY (`promotion_id`) REFERENCES `promotions` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_campaign_budgets` ADD CONSTRAINT `fk_source_a9a7333b1e954fc49be15c08` FOREIGN KEY (`campaign_id`) REFERENCES `promotion_campaigns` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_budget_usages` ADD CONSTRAINT `fk_source_95daed9f0c2ef11d61cc4558` FOREIGN KEY (`budget_id`) REFERENCES `promotion_campaign_budgets` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_order_rules` ADD CONSTRAINT `fk_source_30e6c1a172b48201f6e74fdc` FOREIGN KEY (`promotion_id`) REFERENCES `promotions` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_order_rules` ADD CONSTRAINT `fk_source_e8b50ee61446cf5a69809b2f` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotion_rule_values` ADD CONSTRAINT `fk_source_5f358760aaa8da8dd9b0cbe6` FOREIGN KEY (`promotion_rule_id`) REFERENCES `promotion_rules` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `promotions` ADD CONSTRAINT `ck_promotion_enum_b4885644232bdeedaf5d7187` CHECK (`type` IN ('standard','buyget'));

ALTER TABLE `promotions` ADD CONSTRAINT `ck_promotion_enum_6d6f4a0152a2f47533828efe` CHECK (`status` IN ('draft','active','inactive'));

ALTER TABLE `promotion_application_methods` ADD CONSTRAINT `ck_promotion_enum_b1f2d8124d7a6ca351d55521` CHECK (`type` IN ('fixed','percentage'));

ALTER TABLE `promotion_application_methods` ADD CONSTRAINT `ck_promotion_enum_91712ab756578800a668d5c1` CHECK (`target_type` IN ('order','shipping_methods','items'));

ALTER TABLE `promotion_application_methods` ADD CONSTRAINT `ck_promotion_enum_60ea9cb851cc69305b3c8051` CHECK (`allocation` IN ('each','across','once'));

ALTER TABLE `promotion_campaign_budgets` ADD CONSTRAINT `ck_promotion_enum_2914d57a4cafa407e4205fd6` CHECK (`type` IN ('spend','usage','use_by_attribute','spend_by_attribute'));

ALTER TABLE `promotion_rules` ADD CONSTRAINT `ck_promotion_enum_83768b8bebc69e1e0c8ce122` CHECK (`operator` IN ('gte','lte','gt','lt','eq','ne','in'));

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Promotion configuration readiness does not authorize cart writes or provider execution.
CREATE TABLE promotion_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO promotion_runtime_schema VALUES (1,1,'37ff9efcdcc877ece309bf464d1468b4ddc8e005920b59314d262c23a8d3001d',TRUE,FALSE);
