ALTER TABLE payment_effect_journal
 ADD COLUMN result_envelope LONGBLOB NULL,
 ADD CONSTRAINT ck_payment_effect_result_size CHECK(result_envelope IS NULL OR OCTET_LENGTH(result_envelope) BETWEEN 34 AND 1048673);
CREATE TABLE cart_payment_refresh_compensations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('staged','completed')),
 manifest_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(manifest_json)),
 manifest_sha256 BINARY(32) NOT NULL,
 result_version BIGINT UNSIGNED NULL,
 created_at DATETIME(6) NOT NULL,
 completed_at DATETIME(6) NULL,
 PRIMARY KEY(cart_id,operation_id),
 CHECK((state='staged' AND result_version IS NULL AND completed_at IS NULL) OR (state='completed' AND result_version IS NOT NULL AND completed_at IS NOT NULL))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
