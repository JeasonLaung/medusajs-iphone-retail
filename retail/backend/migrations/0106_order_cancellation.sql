-- A whole-order cancellation is durable independently of its provider refunds.
-- Earlier workers cannot understand cancellation refunds and must fail closed.
CREATE TABLE order_cancellation_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN(FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO order_cancellation_runtime VALUES(1,1,FALSE);
CREATE TABLE order_cancellations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_sha256 BINARY(32) NOT NULL,
 expected_version INT NOT NULL CHECK(expected_version>0),
 reason VARCHAR(1000) NOT NULL,
 completed_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
ALTER TABLE order_refund_runtime DROP CHECK ck_order_refund_order_payment_runtime;
UPDATE order_refund_runtime SET version=5 WHERE id=1;
ALTER TABLE order_refund_runtime ADD CONSTRAINT ck_order_refund_cancellation_runtime CHECK(version=5);
ALTER TABLE order_refund_operations
 MODIFY request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
 ADD COLUMN cancellation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD CONSTRAINT fk_refund_order_cancellation FOREIGN KEY(cancellation_id) REFERENCES order_cancellations(operation_id),
 ADD CONSTRAINT ck_refund_purpose CHECK((request_id IS NOT NULL AND cancellation_id IS NULL) OR (request_id IS NULL AND cancellation_id IS NOT NULL)),
 ADD COLUMN active_cancellation_payment VARCHAR(230) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN cancellation_id IS NOT NULL AND canceled_at IS NULL AND state NOT IN ('failed','canceled') THEN CONCAT(cancellation_id,':',payment_id) ELSE NULL END) STORED,
 ADD UNIQUE INDEX refund_one_active_cancellation_payment(active_cancellation_payment);
