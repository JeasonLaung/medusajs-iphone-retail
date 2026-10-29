ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v9;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v10 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','price.write','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('order.allocation.write');
UPDATE authorization_runtime_schema SET model_sha256='a76ecdc0d6a7b657aa55197651fff00a98016e6d2bf36ce3a85cc5ee6f9e5e65',revision=revision+1 WHERE id=1;
CREATE TABLE order_device_allocation_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO order_device_allocation_runtime VALUES(1,1,FALSE);
CREATE TABLE order_device_allocations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 item_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
