-- Explicit refund authority. Existing roles receive no new grants.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v13;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v14 CHECK(code IN ('authorization.read','authorization.write','buyback.read','buyback.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.after_sales.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','order.refund.write','price.read','price.write','product.read','product.write','warehouse.allocation.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.shipment.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('order.refund.write');
UPDATE authorization_runtime_schema SET model_sha256='7a30b668d78ad4dd28a453cf41c731f87c29fcf72b461c6150ebfe6b6df3b795',revision=revision+1 WHERE id=1;
CREATE TABLE order_refund_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO order_refund_runtime VALUES(1,1,FALSE);
CREATE TABLE order_refund_operations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 payment_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 collection_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 amount VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 currency_code CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 input_sha256 BINARY(32) NOT NULL,
 scope_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 checkout_operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 session_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 provider_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 remote_payment_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 remote_refund_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('pending','uncertain','queued','processing','refunded','failed','canceled')),
 dispatched_at DATETIME(6) NULL,
 lease_until DATETIME(6) NULL,
 claim_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 observation LONGTEXT NULL CHECK(observation IS NULL OR JSON_VALID(observation)),
 applied BOOLEAN NOT NULL DEFAULT FALSE CHECK(applied IN(FALSE,TRUE)),
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(request_id) REFERENCES after_sales_requests(id),
 FOREIGN KEY(payment_id) REFERENCES payments(id),
 UNIQUE KEY refund_remote_identity(provider_id,remote_refund_id),
 INDEX refund_payment_holds(payment_id,state),
 INDEX refund_recovery(state,lease_until),
 CHECK((dispatched_at IS NULL AND lease_until IS NULL AND state='pending' AND observation IS NULL AND applied=FALSE) OR dispatched_at IS NOT NULL),
 CHECK(applied=FALSE OR state='refunded')
) ENGINE=InnoDB;
