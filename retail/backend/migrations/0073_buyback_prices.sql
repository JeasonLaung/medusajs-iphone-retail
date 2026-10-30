-- Historical price/configuration data, independent of login identities.
-- Capacity is retained verbatim; an active digest indexes the original partial
-- product/capacity uniqueness without truncating a source LONGTEXT value.
CREATE TABLE buyback_prices (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 capacity LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 amount DECIMAL(38,18) NOT NULL,
 raw_amount LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 pricing_rules LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
 currency_code VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 active_capacity_digest BINARY(32) GENERATED ALWAYS AS (IF(deleted_at IS NULL,UNHEX(SHA2(capacity,256)),NULL)) STORED,
 UNIQUE KEY buyback_product_capacity(product_id,active_capacity_digest),
 INDEX buyback_price_time(product_id,deleted_at,created_at,id),
 FOREIGN KEY(product_id) REFERENCES catalog_products(id),
 CHECK(JSON_VALID(raw_amount)),
 CHECK(pricing_rules IS NULL OR JSON_VALID(pricing_rules))
) ENGINE=InnoDB;
