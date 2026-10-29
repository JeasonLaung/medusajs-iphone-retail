-- Internal paid-order coordinator; explicitly enabled only after configuration.
CREATE TABLE cart_completion_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN(FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_completion_runtime VALUES(1,1,FALSE);
CREATE TABLE cart_completion_receipts (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_version BIGINT UNSIGNED NOT NULL,
 completed_version BIGINT UNSIGNED NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 payment_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 capture_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 completed_at DATETIME(6) NOT NULL,
 FOREIGN KEY(cart_id,operation_id) REFERENCES cart_checkout_attempts(cart_id,operation_id),
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(payment_id) REFERENCES payments(id),
 FOREIGN KEY(capture_id) REFERENCES payment_captures(id)
) ENGINE=InnoDB;
-- Contains private recipient data ONLY in a row-bound authenticated envelope.
-- No worker is implicitly started and no delivery success is implied by this row.
CREATE TABLE order_notification_outbox (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 event VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(event='order.placed'),
 payload_envelope LONGBLOB NOT NULL,
 status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(status IN('pending','sending','sent','failed')),
 attempts INT UNSIGNED NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL,
 UNIQUE(order_id,event), FOREIGN KEY(order_id) REFERENCES orders(id)
) ENGINE=InnoDB;
