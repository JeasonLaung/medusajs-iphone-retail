-- First-payment checkout attempts bind current cart/version and durable effects.
ALTER TABLE cart_ownership ADD COLUMN checkout_attempt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL;
CREATE TABLE cart_checkout_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN(FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_checkout_runtime VALUES(1,1,FALSE);
CREATE TABLE cart_checkout_attempts (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_version BIGINT UNSIGNED NOT NULL,
 cart_version BIGINT UNSIGNED NOT NULL,
 collection_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 session_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 cart_amount LONGTEXT CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 settlement_amount VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 currency_code VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 activated BOOLEAN NOT NULL DEFAULT FALSE CHECK(activated IN(FALSE,TRUE)),
 request_sha256 BINARY(32) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(cart_id,operation_id), UNIQUE KEY session_identity(session_id),
 CONSTRAINT fk_checkout_owner FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id),
 CHECK(input_version>0 AND cart_version=input_version+1)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
