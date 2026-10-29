ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v7;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v8 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('warehouse.intake.read'),('warehouse.intake.write'),('warehouse.return.read'),('warehouse.return.write');
UPDATE authorization_runtime_schema SET model_sha256='10e78cdc98fa65a573111d95b726fa062cca66a1d787006cb1b2c9ca92bad7c5',revision=revision+1 WHERE id=1;
CREATE TABLE warehouse_device_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 location_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 category VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(category IN ('sale','returned')),
 device_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
