-- Device facts copied from the reviewed relational model; no old login bindings.
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

CREATE TABLE device_runtime_schema (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0),
 identity_enabled BOOLEAN NOT NULL CHECK(identity_enabled IN (0,1)),
 commands_enabled BOOLEAN NOT NULL CHECK(commands_enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO device_runtime_schema VALUES(1,1,1,FALSE,FALSE);
CREATE TABLE device_identity_claims (
 kind VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(kind IN ('serial','radio')),
 value VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 device_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 field VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(field IN ('serial_number','imei','imei2','eid')),
 PRIMARY KEY(kind,value),
 UNIQUE KEY uq_device_identity_field(device_id,field),
 FOREIGN KEY(device_id) REFERENCES devices(id),
 CHECK((kind='serial' AND field='serial_number') OR (kind='radio' AND field IN ('imei','imei2','eid')))
) ENGINE=InnoDB;
