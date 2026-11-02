-- New accounts only. Never populate the legacy carts.customer_id relation.
CREATE TABLE customer_cart_runtime (
 id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
 version INT UNSIGNED NOT NULL,
 enabled BOOLEAN NOT NULL,
 CHECK(id=1)
) ENGINE=InnoDB;
INSERT INTO customer_cart_runtime(id,version,enabled) VALUES(1,1,FALSE);
CREATE TABLE customer_cart_ownership (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 store_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id),
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
CREATE TABLE customer_cart_operations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_version BIGINT UNSIGNED NOT NULL,
 result_version BIGINT UNSIGNED NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(cart_id,operation_id),
 FOREIGN KEY(cart_id) REFERENCES customer_cart_ownership(cart_id),
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
