-- Independent PayPal settlement effect. No account/credential import or runtime enablement.
CREATE TABLE cart_paypal_settlements (
 session_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 checkout_operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 remote_order_id VARCHAR(17) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 intent VARCHAR(9) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(intent IN ('CAPTURE','AUTHORIZE')),
 provider_request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('inflight','uncertain','succeeded')),
 lease_until DATETIME(6) NOT NULL,
 result_envelope LONGBLOB NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 UNIQUE KEY remote_identity(provider_id,remote_order_id),
 UNIQUE KEY request_identity(provider_request_id),
 CONSTRAINT fk_paypal_settlement_checkout FOREIGN KEY(cart_id,checkout_operation_id) REFERENCES cart_checkout_attempts(cart_id,operation_id),
 CHECK((state='succeeded' AND result_envelope IS NOT NULL) OR (state<>'succeeded' AND result_envelope IS NULL))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
