-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE payment_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO payment_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN ('payment_account_holders','payment_captures','payment_collection_providers','payment_collections','payment_refund_reasons','payment_refunds','payment_runtime_schema','payment_sessions','payments'));

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

CREATE INDEX `idx_source_f6a03a9c77a0a750f40867b4` ON `payment_account_holders` (`deleted_at`);

CREATE INDEX `idx_source_cc67305c791f4262650bfe94` ON `payment_captures` (`deleted_at`);

CREATE INDEX `idx_source_52b7020667a98d464a344fe8` ON `payment_captures` (`payment_id`);

CREATE INDEX `idx_source_b711b1af669676f5db0c04c5` ON `payments` (`deleted_at`);

CREATE INDEX `idx_source_bc086049f3f2d3050171d353` ON `payments` (`payment_collection_id`);

CREATE INDEX `idx_source_b787c6d7b956f8db4578fd64` ON `payments` (`payment_session_id`);

ALTER TABLE `payments` ADD COLUMN `active_key_ab1b6a4ef5fc2e255bd0cdb4` TINYINT GENERATED ALWAYS AS (IF(`deleted_at` IS NULL,1,NULL)) STORED;

CREATE UNIQUE INDEX `idx_source_ab1b6a4ef5fc2e255bd0cdb4` ON `payments` (`payment_session_id`,`active_key_ab1b6a4ef5fc2e255bd0cdb4`);

CREATE INDEX `idx_source_c7bd078f9d675258c1a376c6` ON `payments` (`provider_id`);

CREATE INDEX `idx_source_a14fe382d304c17e31c2d0fb` ON `payment_collections` (`deleted_at`);

CREATE INDEX `idx_source_b9248659997fb45196ae7980` ON `payment_sessions` (`deleted_at`);

CREATE INDEX `idx_source_42b8abed96c928be0b8c439c` ON `payment_sessions` (`payment_collection_id`);

CREATE INDEX `idx_source_b27f0c601d1de8b6965ac1a7` ON `payment_refunds` (`deleted_at`);

CREATE INDEX `idx_source_0f620d75004790db3ca9ec8a` ON `payment_refunds` (`payment_id`);

CREATE INDEX `idx_source_499252ef124ea789379c85b5` ON `payment_refunds` (`refund_reason_id`);

CREATE INDEX `idx_source_fc74b4959361109633a9b78d` ON `payment_refund_reasons` (`deleted_at`);

ALTER TABLE `payment_captures` ADD CONSTRAINT `fk_source_d53cde0eeccbb90eb9fd6125` FOREIGN KEY (`payment_id`) REFERENCES `payments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payments` ADD CONSTRAINT `fk_source_5ecd7def6ee14d7ec86997a3` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_collection_providers` ADD CONSTRAINT `fk_source_1b669e4c5a658caa85ac4151` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_collection_providers` ADD CONSTRAINT `fk_source_4e01fa660b250025e928ce6a` FOREIGN KEY (`payment_provider_id`) REFERENCES `payment_providers` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_sessions` ADD CONSTRAINT `fk_source_cd417994fcbf48dc086631c9` FOREIGN KEY (`payment_collection_id`) REFERENCES `payment_collections` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_refunds` ADD CONSTRAINT `fk_source_1873177dfc20cf7bd44b1606` FOREIGN KEY (`payment_id`) REFERENCES `payments` (`id`) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE `payment_collections` ADD CONSTRAINT `ck_payment_enum_59f5f920ad60f976400bd90e` CHECK (`status` IN ('not_paid','awaiting','authorized','partially_authorized','canceled','failed','partially_captured','completed'));

ALTER TABLE `payment_sessions` ADD CONSTRAINT `ck_payment_enum_5649e46ef120097537207d07` CHECK (`status` IN ('authorized','captured','pending','requires_more','error','canceled','pending_authorization'));

-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Historical payment actors and ciphertext do not grant login or provider authority.
CREATE TABLE payment_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO payment_runtime_schema VALUES (1,1,'bf2b1d2d79e1797ef958a9f7c4c3b75fa4be67f048d84653653a338443043819',TRUE,FALSE);
