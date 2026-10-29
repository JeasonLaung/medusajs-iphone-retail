-- Internal refresh coordination is separately gated, closed by default.
ALTER TABLE cart_ownership ADD COLUMN payment_refresh_operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL;
CREATE TABLE cart_payment_refresh_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_payment_refresh_runtime VALUES(1,1,FALSE);
CREATE TABLE cart_payment_refresh_operations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cart_version BIGINT UNSIGNED NOT NULL CHECK(cart_version>0),
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('staged','completed')),
 manifest_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(manifest_json)),
 manifest_sha256 BINARY(32) NOT NULL,
 result_version BIGINT UNSIGNED NULL,
 created_at DATETIME(6) NOT NULL,
 completed_at DATETIME(6) NULL,
 PRIMARY KEY(cart_id,operation_id),
 INDEX recovery(state,created_at),
 CHECK((state='staged' AND result_version IS NULL AND completed_at IS NULL) OR (state='completed' AND result_version IS NOT NULL AND completed_at IS NOT NULL))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
