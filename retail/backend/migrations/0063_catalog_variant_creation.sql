CREATE TABLE catalog_variant_create_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1)),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0),
 last_sku_number BIGINT NOT NULL CHECK(last_sku_number>=0)
) ENGINE=InnoDB;
INSERT INTO catalog_variant_create_runtime VALUES(1,1,FALSE,1,0);
CREATE TABLE catalog_variant_creations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 variant_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 inventory_item_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 sku VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
