-- Native order supplemental payment authority. No historical backfill or enable.
CREATE TABLE order_payment_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN(FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO order_payment_runtime VALUES(1,1,FALSE);
CREATE TABLE order_payment_operations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 previous_operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL UNIQUE,
 account_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 auth_version BIGINT UNSIGNED NOT NULL CHECK(auth_version>0),
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 order_version INT NOT NULL CHECK(order_version>0),
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 collection_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 session_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 amount VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 currency_code CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 provider_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 input_sha256 BINARY(32) NOT NULL, request_sha256 BINARY(32) NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN('pending','uncertain','open','authorized','paid','failed','canceled','expired')),
 dispatched_at DATETIME(6) NULL,
 applied BOOLEAN NOT NULL DEFAULT FALSE CHECK(applied IN(FALSE,TRUE)),
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 active_collection VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN state NOT IN('failed','canceled','expired') THEN collection_id ELSE NULL END) STORED,
 active_order VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN state NOT IN('failed','canceled','expired') AND applied=FALSE THEN order_id ELSE NULL END) STORED,
 UNIQUE KEY order_payment_active_collection(active_collection),
 UNIQUE KEY order_payment_active_order(active_order),
 INDEX order_payment_history(order_id,created_at,operation_id),
 INDEX order_payment_recovery(state,updated_at),
 FOREIGN KEY(previous_operation_id) REFERENCES order_payment_operations(operation_id),
 FOREIGN KEY(account_id) REFERENCES customer_accounts(id),
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(cart_id) REFERENCES cart_completion_receipts(cart_id),
 FOREIGN KEY(collection_id) REFERENCES payment_collections(id),
 FOREIGN KEY(session_id) REFERENCES payment_sessions(id),
 CHECK(applied=FALSE OR (state='paid' AND dispatched_at IS NOT NULL)),
 CHECK(dispatched_at IS NOT NULL OR state IN('pending','canceled'))
) ENGINE=InnoDB;
CREATE TABLE order_payment_cancellations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 payment_operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 account_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 auth_version BIGINT UNSIGNED NOT NULL CHECK(auth_version>0),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(payment_operation_id) REFERENCES order_payment_operations(operation_id),
 FOREIGN KEY(account_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
-- Source evidence stays model v1. Native contract v2 adds the order-wide payment
-- hold. Old order writers/importers checking runtime v1 must fail closed.
ALTER TABLE order_runtime_schema DROP CHECK order_runtime_schema_chk_2;
UPDATE order_runtime_schema SET schema_version=2 WHERE id=1;
ALTER TABLE order_runtime_schema ADD CONSTRAINT ck_order_native_payment_runtime CHECK(schema_version=2);

-- Old refund workers do not honor supplemental payment holds. Preserve enable.
ALTER TABLE order_refund_runtime DROP CHECK ck_order_refund_exchange_runtime;
UPDATE order_refund_runtime SET version=4 WHERE id=1;
ALTER TABLE order_refund_runtime ADD CONSTRAINT ck_order_refund_order_payment_runtime CHECK(version=4);
