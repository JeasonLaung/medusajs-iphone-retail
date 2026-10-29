-- Verified return observations remain separate from immutable creation results.
ALTER TABLE cart_checkout_attempts
 ADD COLUMN observed_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN needs_review BOOLEAN NOT NULL DEFAULT FALSE CHECK(needs_review IN(FALSE,TRUE));
CREATE TABLE cart_checkout_observations (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 result_envelope LONGBLOB NOT NULL,
 created_at DATETIME(6) NOT NULL,
 INDEX attempt_observations(cart_id,operation_id,created_at),
 CONSTRAINT fk_checkout_observation_attempt FOREIGN KEY(cart_id,operation_id) REFERENCES cart_checkout_attempts(cart_id,operation_id)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
