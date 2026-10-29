-- Explicit shipping selection command gate and non-PII immutable receipts.
CREATE TABLE cart_shipping_command_runtime (
 id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
 version INT UNSIGNED NOT NULL,
 enabled BOOLEAN NOT NULL,
 CONSTRAINT ck_cart_shipping_command_singleton CHECK (id=1)
) ENGINE=InnoDB;
INSERT INTO cart_shipping_command_runtime(id,version,enabled) VALUES(1,1,FALSE);
CREATE TABLE cart_shipping_command_operations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 result_version BIGINT UNSIGNED NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(cart_id,operation_id),
 CONSTRAINT fk_cart_shipping_command_owner FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id)
) ENGINE=InnoDB;
